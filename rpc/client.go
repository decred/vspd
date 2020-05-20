package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	wallettypes "decred.org/dcrwallet/rpc/jsonrpc/types"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/jrick/wsrpc/v2"
)

type Client func() (*wsrpc.Client, error)

const (
	requiredWalletVersion = "8.1.0"
)

// Setup accepts RPC connection details, creates an RPC client, and returns a
// function which can be called to access the client. The returned function will
// try to handle any client disconnects by attempting to reconnect, but will
// return an error if a new connection cannot be established.
func Setup(ctx context.Context, shutdownWg *sync.WaitGroup, user, pass, addr string, cert []byte) Client {

	// Create TLS options.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cert)
	tc := &tls.Config{RootCAs: pool}
	tlsOpt := wsrpc.WithTLSConfig(tc)

	// Create authentication options.
	authOpt := wsrpc.WithBasicAuth(user, pass)

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

	return func() (*wsrpc.Client, error) {
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

		fullAddr := "wss://" + addr + "/ws"
		c, err := wsrpc.Dial(ctx, fullAddr, tlsOpt, authOpt)
		if err != nil {
			return nil, err
		}
		log.Infof("Dialed RPC websocket %v", addr)

		// Verify dcrwallet at is at the required api version
		var verMap map[string]dcrdtypes.VersionResult
		err = c.Call(ctx, "version", &verMap)
		if err != nil {
			c.Close()
			return nil, fmt.Errorf("wallet %v version failed: %v",
				addr, err)
		}
		walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
		if !exists {
			c.Close()
			return nil, fmt.Errorf("wallet %v version response "+
				"missing 'dcrwalletjsonrpcapi'", addr)
		}
		if walletVersion.VersionString != requiredWalletVersion {
			c.Close()
			return nil, fmt.Errorf("wallet %v is not at the "+
				"proper version: %s != %s", addr,
				walletVersion.VersionString, requiredWalletVersion)
		}

		// Verify dcrwallet is voting
		var walletInfo wallettypes.WalletInfoResult
		err = c.Call(ctx, "walletinfo", &walletInfo)
		if err != nil {
			c.Close()
			return nil, err
		}
		if !walletInfo.Voting || !walletInfo.Unlocked {
			c.Close()
			return nil, fmt.Errorf("wallet %s has voting disabled", addr)
		}

		return c, nil
	}
}
