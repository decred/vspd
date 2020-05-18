package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/webapi"
	"github.com/jrick/wsrpc/v2"
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

	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		// Don't use logger here because it may not be initialised.
		fmt.Fprintf(os.Stderr, "Config error: %v", err)
		return err
	}

	// Waitgroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup
	defer log.Info("Shutdown complete")

	// Open database.
	db, err := database.Open(ctx, &shutdownWg, cfg.dbPath)
	if err != nil {
		log.Errorf("Database error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// TODO: Create real RPC client.
	var rpc *wsrpc.Client

	// Create and start webapi server.
	apiCfg := webapi.Config{
		SignKey:   cfg.signKey,
		PubKey:    cfg.pubKey,
		VSPFee:    cfg.VSPFee,
		NetParams: cfg.netParams.Params,
	}
	// TODO: Make releaseMode properly configurable. Release mode enables very
	// detailed webserver logging and live reloading of HTML templates.
	releaseMode := true
	err = webapi.Start(ctx, shutdownRequestChannel, &shutdownWg, cfg.Listen, db, rpc, releaseMode, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialise webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Wait for shutdown tasks to complete before returning.
	shutdownWg.Wait()

	return ctx.Err()
}
