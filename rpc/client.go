// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"

	"github.com/decred/slog"
	"github.com/jrick/wsrpc/v2"
)

// Caller provides a client interface to perform JSON-RPC remote procedure calls.
type Caller interface {
	// String returns the dialed URL.
	String() string

	// Call performs the remote procedure call defined by method and
	// waits for a response or a broken client connection.
	// Args provides positional parameters for the call.
	// Res must be a pointer to a struct, slice, or map type to unmarshal
	// a result (if any), or nil if no result is needed.
	Call(ctx context.Context, method string, res interface{}, args ...interface{}) error
}

// client wraps a wsrpc.Client, as well as all of the connection details
// required to make a new client if the existing client is closed.
type client struct {
	mu       *sync.Mutex
	client   *wsrpc.Client
	addr     string
	tlsOpt   wsrpc.Option
	authOpt  wsrpc.Option
	notifier wsrpc.Notifier
	log      slog.Logger
}

func setup(user, pass, addr string, cert []byte, log slog.Logger) *client {

	// Create TLS options.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(cert)
	tc := &tls.Config{
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		CipherSuites: []uint16{ // Only applies to TLS 1.2. TLS 1.3 ciphersuites are not configurable.
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		RootCAs: pool,
	}
	tlsOpt := wsrpc.WithTLSConfig(tc)

	// Create authentication options.
	authOpt := wsrpc.WithBasicAuth(user, pass)

	var mu sync.Mutex
	var c *wsrpc.Client
	fullAddr := "wss://" + addr + "/ws"
	return &client{&mu, c, fullAddr, tlsOpt, authOpt, nil, log}
}

func (c *client) Close() {
	if c.client != nil {
		select {
		case <-c.client.Done():
			c.log.Tracef("RPC already closed (%s)", c.addr)

		default:
			if err := c.client.Close(); err != nil {
				c.log.Errorf("Failed to close RPC (%s): %v", c.addr, err)
			} else {
				c.log.Tracef("RPC closed (%s)", c.addr)
			}
		}
	}
}

// dial will return a connect rpc client if one exists, or attempt to create a
// new one if not. A boolean indicates whether this connection is new (true), or
// if it is an existing connection which is being reused (false).
func (c *client) dial(ctx context.Context) (Caller, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		select {
		case <-c.client.Done():
			c.log.Debugf("RPC client %s errored (%v); reconnecting...", c.addr, c.client.Err())
			c.client = nil
		default:
			return c.client, false, nil
		}
	}

	var err error
	c.client, err = wsrpc.Dial(ctx, c.addr, c.tlsOpt, c.authOpt, wsrpc.WithNotifier(c.notifier))
	if err != nil {
		return nil, false, err
	}
	return c.client, true, nil
}
