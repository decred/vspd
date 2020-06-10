package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/decred/vspd/background"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/webapi"
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
		// Don't use logger here because it may not be initialized.
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		return err
	}

	if cfg.VspClosed {
		log.Warnf("Config --vspclosed is set. This will prevent vspd from " +
			"accepting new tickets.")
	}

	// Waitgroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup
	defer log.Info("Shutdown complete")

	// Open database.
	db, err := database.Open(ctx, &shutdownWg, cfg.dbPath, cfg.BackupInterval)
	if err != nil {
		log.Errorf("Database error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	defer db.Close()

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, nil)
	defer dcrd.Close()

	// Create RPC client for remote dcrwallet instance (used for voting).
	wallets := rpc.SetupWallet(cfg.WalletUser, cfg.WalletPass, cfg.WalletHosts, cfg.walletCert)
	defer wallets.Close()

	// Create and start webapi server.
	apiCfg := webapi.Config{
		VSPFee:       cfg.VSPFee,
		NetParams:    cfg.netParams.Params,
		SupportEmail: cfg.SupportEmail,
		VspClosed:    cfg.VspClosed,
		AdminPass:    cfg.AdminPass,
	}
	err = webapi.Start(ctx, shutdownRequestChannel, &shutdownWg, cfg.Listen, db,
		dcrd, wallets, cfg.WebServerDebug, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialize webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create a dcrd client with a blockconnected notification handler.
	dcrdWithNotifs := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass,
		cfg.DcrdHost, cfg.dcrdCert, &background.NotificationHandler{})
	defer dcrdWithNotifs.Close()

	// Start background process which will continually attempt to reconnect to
	// dcrd if the connection drops.
	background.Start(ctx, &shutdownWg, db, dcrd, dcrdWithNotifs, wallets, cfg.netParams.Params)

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return ctx.Err()
}
