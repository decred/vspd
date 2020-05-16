package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jrick/wsrpc/v2"
)

var (
	cfg            *config
	db             *database.VspDatabase
	nodeConnection *wsrpc.Client
)

func main() {
	// Create a context that is cancelled when a shutdown request is received
	// through an interrupt signal.
	ctx := withShutdownCancel(context.Background())
	go shutdownListener()

	// Run until error is returned, or shutdown is requested.
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
}

// run is the main startup and teardown logic performed by the main package. It
// is responsible for parsing the config, creating a dcrwallet RPC client,
// opening the database, starting the webserver, and stopping all started
// services when the context is cancelled.
func run(ctx context.Context) error {
	var err error

	// Load config file and parse CLI args.
	cfg, err = loadConfig()
	if err != nil {
		// Don't use logger here because it may not be initialised.
		fmt.Fprintf(os.Stderr, "Config error: %v", err)
		return err
	}

	// Open database.
	db, err = database.New(cfg.dbPath)
	if err != nil {
		log.Errorf("Database error: %v", err)
		return err
	}
	// Close database.
	defer func() {
		log.Debug("Closing database...")
		err := db.Close()
		if err != nil {
			log.Errorf("Error closing database: %v", err)
		} else {
			log.Debug("Database closed")
		}
	}()

	// Create TCP listener for webserver.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		log.Errorf("Failed to create tcp listener: %v", err)
		return err
	}
	log.Infof("Listening on %s", cfg.Listen)

	// Create webserver.
	// TODO: Make releaseMode properly configurable.
	releaseMode := true
	srv := http.Server{
		Handler:      newRouter(releaseMode),
		ReadTimeout:  5 * time.Second,  // slow requests should not hold connections opened
		WriteTimeout: 60 * time.Second, // hung responses must die
	}

	// Start webserver.
	go func() {
		err = srv.Serve(listener)
		// If the server dies for any reason other than ErrServerClosed (from
		// graceful server.Shutdown), log the error and request dcrvsp be
		// shutdown.
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Unexpected webserver error: %v", err)
			requestShutdown()
		}
	}()

	// Stop webserver.
	defer func() {
		log.Debug("Stopping webserver...")
		// Give the webserver 5 seconds to finish what it is doing.
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(timeoutCtx); err != nil {
			log.Errorf("Failed to stop webserver cleanly: %v", err)
		}
		log.Debug("Webserver stopped")
	}()

	// Wait until shutdown is signaled before returning and running deferred
	// shutdown tasks.
	<-ctx.Done()

	return ctx.Err()
}
