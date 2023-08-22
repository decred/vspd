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

// shutdownRequestChannel is used to initiate shutdown from one of the
// subsystems using the same code paths as when an interrupt signal is received.
var shutdownRequestChannel = make(chan struct{})

// interruptSignals defines the signals that are handled to do a clean shutdown.
// Conditional compilation is used to also include SIGTERM and SIGHUP on Unix.
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
		select {
		case sig := <-interruptChannel:
			log.Infof("Received signal (%s). Shutting down...", sig)
		case <-shutdownRequestChannel:
			log.Info("Shutdown requested. Shutting down...")
		}

		cancel()

		// Listen for any more shutdown request and display a message so the
		// user knows the shutdown is in progress and the process is not hung.
		for {
			select {
			case sig := <-interruptChannel:
				log.Infof("Received signal (%s). Already shutting down...", sig)
			case <-shutdownRequestChannel:
				log.Info("Shutdown requested. Already shutting down...")
			}
		}
	}()
	return ctx
}

// requestShutdown signals for starting the clean shutdown of the process
// through an internal component.
func requestShutdown() {
	shutdownRequestChannel <- struct{}{}
}
