package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
	"github.com/jholdstock/dcrvsp/webapi"
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
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
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

	// Create dcrwallet RPC client.
	walletRPC := rpc.Setup(ctx, &shutdownWg, cfg.WalletUser, cfg.WalletPass, cfg.WalletHost, cfg.dcrwCert)
	walletClient, err := walletRPC()
	if err != nil {
		log.Errorf("dcrwallet RPC error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	signKey, pubKey, err := db.KeyPair()
	if err != nil {
		log.Errorf("Failed to get keypair: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
  
	// Get the masterpubkey from the fees account, if it exists, and make
	// sure it matches the configuration.
	var existingXPub string
	err = walletClient.Call(ctx, "getmasterpubkey", &existingXPub, "fees")
	if err != nil {
		// TODO - ignore account not found
		log.Errorf("dcrwallet RPC error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}


		// account exists - make sure it matches the configuration.
		if existingXPub != cfg.FeeXPub {
			log.Errorf("fees account xpub differs: %s != %s", existingXPub, cfg.FeeXPub)
			requestShutdown()
			shutdownWg.Wait()
			return err
	} else {
		// account does not exist - import xpub from configuration.
		if err = walletClient.Call(ctx, "importxpub", nil, "fees"); err != nil {
			log.Errorf("failed to import xpub: %v", err)
			requestShutdown()
			shutdownWg.Wait()
			return err
		}
 }

	// Create and start webapi server.
	apiCfg := webapi.Config{
		SignKey:   signKey,
		PubKey:    pubKey,
		VSPFee:    cfg.VSPFee,
		NetParams: cfg.netParams.Params,
	}
	// TODO: Make releaseMode properly configurable. Release mode enables very
	// detailed webserver logging and live reloading of HTML templates.
	releaseMode := true
	err = webapi.Start(ctx, shutdownRequestChannel, &shutdownWg, cfg.Listen, db, walletRPC, releaseMode, apiCfg)
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
