// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"

	"github.com/decred/vspd/background"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/version"
	"github.com/decred/vspd/webapi"
)

// maxVoteChangeRecords defines how many vote change records will be stored for
// each ticket. The limit is in place to mitigate DoS attacks on server storage
// space. When storing a new record breaches this limit, the oldest record in
// the database is deleted.
const maxVoteChangeRecords = 10

func main() {
	// Create a context that is cancelled when a shutdown request is received
	// through an interrupt signal.
	shutdownCtx := withShutdownCancel(context.Background())
	go shutdownListener()

	// Run until error is returned, or shutdown is requested.
	if err := run(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
}

// run is the main startup and teardown logic performed by the main package. It
// is responsible for parsing the config, creating a dcrwallet RPC client,
// opening the database, starting the webserver, and stopping all started
// services when the context is cancelled.
func run(shutdownCtx context.Context) error {

	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		// Don't use logger here because it may not be initialized.
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		return err
	}

	// Show version at startup.
	log.Criticalf("Version %s (Go version %s %s/%s)", version.String(), runtime.Version(),
		runtime.GOOS, runtime.GOARCH)

	if cfg.netParams == &mainNetParams &&
		version.PreRelease != "" {
		log.Warnf("")
		log.Warnf("\tWARNING: This is a pre-release version of vspd which should not be used on mainnet.")
		log.Warnf("")
	}

	if cfg.VspClosed {
		log.Warnf("")
		log.Warnf("\tWARNING: Config --vspclosed is set. This will prevent vspd from accepting new tickets.")
		log.Warnf("")
	}

	// WaitGroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup
	defer log.Criticalf("Shutdown complete")

	// Open database.
	db, err := database.Open(shutdownCtx, &shutdownWg, cfg.dbPath, cfg.BackupInterval, maxVoteChangeRecords)
	if err != nil {
		log.Errorf("Database error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	defer db.Close()

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, nil, cfg.netParams.Params)
	defer dcrd.Close()

	// Create RPC client for remote dcrwallet instance (used for voting).
	wallets := rpc.SetupWallet(cfg.walletUsers, cfg.walletPasswords, cfg.walletHosts, cfg.walletCerts, cfg.netParams.Params)
	defer wallets.Close()

	// Ensure all data in database is present and up-to-date.
	err = db.CheckIntegrity(dcrd)
	if err != nil {
		// vspd should still start if this fails, so just log an error.
		log.Errorf("Could not check database integrity: %v", err)
	}

	// Create and start webapi server.
	apiCfg := webapi.Config{
		VSPFee:               cfg.VSPFee,
		NetParams:            cfg.netParams.Params,
		BlockExplorerURL:     cfg.netParams.BlockExplorerURL,
		SupportEmail:         cfg.SupportEmail,
		VspClosed:            cfg.VspClosed,
		VspClosedMsg:         cfg.VspClosedMsg,
		AdminPass:            cfg.AdminPass,
		Debug:                cfg.WebServerDebug,
		Designation:          cfg.Designation,
		MaxVoteChangeRecords: maxVoteChangeRecords,
		VspdVersion:          version.String(),
	}
	err = webapi.Start(shutdownCtx, requestShutdown, &shutdownWg, cfg.Listen, db,
		dcrd, wallets, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialize webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create a dcrd client with a blockconnected notification handler.
	notifHandler := background.NotificationHandler{ShutdownWg: &shutdownWg}
	dcrdWithNotifs := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass,
		cfg.DcrdHost, cfg.dcrdCert, &notifHandler, cfg.netParams.Params)
	defer dcrdWithNotifs.Close()

	// Start background process which will continually attempt to reconnect to
	// dcrd if the connection drops.
	background.Start(shutdownCtx, &shutdownWg, db, dcrd, dcrdWithNotifs, wallets)

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return shutdownCtx.Err()
}
