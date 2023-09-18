// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/decred/slog"
)

// interruptSignals defines the default signals to catch in order to do a proper
// shutdown. This may be modified during init depending on the platform.
var interruptSignals = []os.Signal{os.Interrupt}

// shutdownListener listens for OS Signals such as SIGINT (Ctrl+C) and shutdown
// requests from requestShutdown. It returns a context that is canceled when
// either signal is received.
func shutdownListener(log slog.Logger) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, interruptSignals...)

		// Listen for the initial shutdown signal.
		sig := <-interruptChannel
		log.Infof("Received signal (%s). Shutting down...", sig)

		cancel()

		// Listen for any more shutdown request and display a message so the
		// user knows the shutdown is in progress and the process is not hung.
		for {
			sig := <-interruptChannel
			log.Infof("Received signal (%s). Already shutting down...", sig)
		}
	}()
	return ctx
}
