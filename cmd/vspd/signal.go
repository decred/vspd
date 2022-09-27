// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2021-2022 The Decred developers
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

// shutdownSignaled is closed whenever shutdown is invoked through an interrupt
// signal or from an JSON-RPC stop request.  Any contexts created using
// withShutdownChannel are cancelled when this is closed.
var shutdownSignaled = make(chan struct{})

// interruptSignals defines the signals that are handled to do a clean shutdown.
// Conditional compilation is used to also include SIGTERM and SIGHUP on Unix.
var interruptSignals = []os.Signal{os.Interrupt}

// withShutdownCancel creates a copy of a context that is cancelled whenever
// shutdown is invoked through an interrupt signal or from an JSON-RPC stop
// request.
func withShutdownCancel(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-shutdownSignaled
		cancel()
	}()
	return ctx
}

// requestShutdown signals for starting the clean shutdown of the process
// through an internal component.
func requestShutdown() {
	shutdownRequestChannel <- struct{}{}
}

// shutdownListener listens for shutdown requests and cancels all contexts
// created from withShutdownCancel.  This function never returns and is intended
// to be spawned in a new goroutine.
func shutdownListener(log slog.Logger) {
	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, interruptSignals...)

	// Listen for the initial shutdown signal
	select {
	case sig := <-interruptChannel:
		log.Infof("Received signal (%s). Shutting down...", sig)
	case <-shutdownRequestChannel:
		log.Info("Shutdown requested. Shutting down...")
	}

	// Cancel all contexts created from withShutdownCancel.
	close(shutdownSignaled)

	// Listen for any more shutdown signals and log that shutdown has already
	// been signaled.
	for {
		select {
		case sig := <-interruptChannel:
			log.Infof("Received signal (%s). Already shutting down...", sig)
		case <-shutdownRequestChannel:
			log.Info("Shutdown requested. Already shutting down...")
		}
	}
}
