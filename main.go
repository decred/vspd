package main

import (
	"context"
	"errors"
	"fmt"
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

	// TODO: Make releaseMode properly configurable.
	releaseMode := true
	router := newRouter(releaseMode)
	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: router,
	}

	// Start webserver.
	log.Infof("Listening on %s", cfg.Listen)
	go srv.ListenAndServe()
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
