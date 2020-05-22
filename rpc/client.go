package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"

	"github.com/jrick/wsrpc/v2"
)

// Caller provides a client interface to perform JSON-RPC remote procedure calls.
type Caller interface {
	// Call performs the remote procedure call defined by method and
	// waits for a response or a broken client connection.
	// Args provides positional parameters for the call.
	// Res must be a pointer to a struct, slice, or map type to unmarshal
	// a result (if any), or nil if no result is needed.
	Call(ctx context.Context, method string, res interface{}, args ...interface{}) error
}

// Connect dials and returns a connected RPC client.
type Connect func() (Caller, error)

// Setup accepts RPC connection details, creates an RPC client, and returns a
// function which can be called to access the client. The returned function will
// try to handle any client disconnects by attempting to reconnect, but will
// return an error if a new connection cannot be established.
func Setup(ctx context.Context, shutdownWg *sync.WaitGroup, user, pass, addr string, cert []byte) Connect {

	// Create TLS options.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cert)
	tc := &tls.Config{RootCAs: pool}
	tlsOpt := wsrpc.WithTLSConfig(tc)

	// Create authentication options.
	authOpt := wsrpc.WithBasicAuth(user, pass)

	fullAddr := "wss://" + addr + "/ws"

	var mu sync.Mutex
	var c *wsrpc.Client

	// Add the graceful shutdown to the waitgroup.
	shutdownWg.Add(1)
	go func() {
		// Wait until shutdown is signaled before shutting down.
		<-ctx.Done()

		if c != nil {
			select {
			case <-c.Done():
				log.Debugf("RPC already closed (%s)", addr)

			default:
				log.Debugf("Closing RPC (%s)...", addr)
				if err := c.Close(); err != nil {
					log.Errorf("Failed to close RPC (%s): %v", addr, err)
				} else {
					log.Debugf("RPC closed (%s)", addr)
				}
			}
		}
		shutdownWg.Done()
	}()

	return func() (Caller, error) {
		defer mu.Unlock()
		mu.Lock()

		if c != nil {
			select {
			case <-c.Done():
				log.Infof("RPC client errored (%v); reconnecting...", c.Err())
				c = nil
			default:
				return c, nil
			}
		}

		var err error
		c, err = wsrpc.Dial(ctx, fullAddr, tlsOpt, authOpt)
		if err != nil {
			return nil, err
		}

		return c, nil
	}
}
