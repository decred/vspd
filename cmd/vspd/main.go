// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/decred/dcrd/wire"
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

// consistencyInterval is the time period between wallet consistency checks.
const consistencyInterval = 30 * time.Minute

func main() {
	// Run until an exit code is returned.
	os.Exit(run())
}

// run is the main startup and teardown logic performed by the main package. It
// is responsible for parsing the config, creating dcrd and dcrwallet RPC clients,
// opening the database, starting the webserver, and stopping all started
// services when a shutdown is requested.
func run() int {

	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		return 1
	}

	log := cfg.logger("VSP")
	dbLog := cfg.logger(" DB")
	apiLog := cfg.logger("API")
	rpcLog := cfg.logger("RPC")

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

	defer log.Criticalf("Shutdown complete")

	// Open database.
	db, err := database.Open(cfg.dbPath, dbLog, maxVoteChangeRecords)
	if err != nil {
		log.Errorf("Database error: %v", err)
		return 1
	}

	const writeBackup = true
	defer db.Close(writeBackup)

	// Create a context that is cancelled when a shutdown request is received
	// through an interrupt signal.
	shutdownCtx := withShutdownCancel(context.Background())
	go shutdownListener(log)

	// WaitGroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup

	db.WritePeriodicBackups(shutdownCtx, &shutdownWg, cfg.BackupInterval)

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, cfg.netParams.Params, rpcLog)
	defer dcrd.Close()

	// Create RPC client for remote dcrwallet instance (used for voting).
	wallets := rpc.SetupWallet(cfg.walletUsers, cfg.walletPasswords, cfg.walletHosts, cfg.walletCerts, cfg.netParams.Params, rpcLog)
	defer wallets.Close()

	// Ensure all data in database is present and up-to-date.
	err = db.CheckIntegrity(dcrd)
	if err != nil {
		// vspd should still start if this fails, so just log an error.
		log.Errorf("Could not check database integrity: %v", err)
	}

	// Run the block connected handler now to catch up with any blocks mined
	// while vspd was shut down.
	blockConnected(dcrd, wallets, db, log)

	// Run voting wallet consistency check now to ensure all wallets are up to
	// date.
	checkWalletConsistency(dcrd, wallets, db, log)

	// Run voting wallet consistency check periodically.
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-shutdownCtx.Done():
				shutdownWg.Done()
				return
			case <-time.After(consistencyInterval):
				checkWalletConsistency(dcrd, wallets, db, log)
			}
		}
	}()

	// Create and start webapi server.
	apiCfg := webapi.Config{
		VSPFee:               cfg.VSPFee,
		NetParams:            cfg.netParams.Params,
		BlockExplorerURL:     cfg.netParams.blockExplorerURL,
		SupportEmail:         cfg.SupportEmail,
		VspClosed:            cfg.VspClosed,
		VspClosedMsg:         cfg.VspClosedMsg,
		AdminPass:            cfg.AdminPass,
		Debug:                cfg.WebServerDebug,
		Designation:          cfg.Designation,
		MaxVoteChangeRecords: maxVoteChangeRecords,
		VspdVersion:          version.String(),
	}
	err = webapi.Start(shutdownCtx, requestShutdown, &shutdownWg, cfg.Listen, db, apiLog,
		dcrd, wallets, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialize webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return 1
	}

	// Create a channel to receive blockConnected notifications from dcrd.
	notifChan := make(chan *wire.BlockHeader)
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-shutdownCtx.Done():
				shutdownWg.Done()
				return
			case header := <-notifChan:
				log.Debugf("Block notification %d (%s)", header.Height, header.BlockHash().String())
				blockConnected(dcrd, wallets, db, log)
			}
		}
	}()

	// Attach notification listener to dcrd client.
	dcrd.BlockConnectedHandler(notifChan)

	// Loop forever attempting ensuring a dcrd connection is available, so
	// notifications are received.
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-shutdownCtx.Done():
				shutdownWg.Done()
				return
			case <-time.After(time.Second * 15):
				// Ensure dcrd client is still connected.
				_, _, err := dcrd.Client()
				if err != nil {
					log.Errorf("dcrd connect error: %v", err)
				}
			}
		}
	}()

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return 0
}
