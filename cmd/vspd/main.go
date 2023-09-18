// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/internal/config"
	"github.com/decred/vspd/internal/version"
	"github.com/decred/vspd/internal/vspd"
	"github.com/decred/vspd/internal/webapi"
	"github.com/decred/vspd/rpc"
)

const (
	// maxVoteChangeRecords defines how many vote change records will be stored
	// for each ticket. The limit is in place to mitigate DoS attacks on server
	// storage space. When storing a new record breaches this limit, the oldest
	// record in the database is deleted.
	maxVoteChangeRecords = 10
)

func main() {
	os.Exit(run())
}

// run is the real main function for vspd. It is necessary to work around the
// fact that deferred functions do not run when os.Exit() is called.
func run() int {
	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadConfig error: %v\n", err)
		return 1
	}

	log := cfg.logger("VSP")

	defer log.Criticalf("Shutdown complete")
	log.Criticalf("Version %s (Go version %s %s/%s)", version.String(),
		runtime.Version(), runtime.GOOS, runtime.GOARCH)

	if cfg.network == &config.MainNet && version.IsPreRelease() {
		log.Warnf("")
		log.Warnf("\tWARNING: This is a pre-release version of vspd which should not be used on mainnet.")
		log.Warnf("")
	}

	if cfg.VspClosed {
		log.Warnf("")
		log.Warnf("\tWARNING: Config --vspclosed is set. This will prevent vspd from accepting new tickets.")
		log.Warnf("")
	}

	// Open database.
	db, err := database.Open(cfg.dbPath, cfg.logger(" DB"), maxVoteChangeRecords)
	if err != nil {
		log.Errorf("Failed to open database: %v", err)
		return 1
	}
	const writeBackup = true
	defer db.Close(writeBackup)

	rpcLog := cfg.logger("RPC")

	// Create a channel to receive blockConnected notifications from dcrd.
	blockNotifChan := make(chan *wire.BlockHeader)

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, cfg.network.Params, rpcLog, blockNotifChan)
	defer dcrd.Close()

	// Create RPC client for remote dcrwallet instances (used for voting).
	wallets := rpc.SetupWallet(cfg.walletUsers, cfg.walletPasswords, cfg.walletHosts, cfg.walletCerts, cfg.network.Params, rpcLog)
	defer wallets.Close()

	// Create webapi server.
	apiCfg := webapi.Config{
		Listen:               cfg.Listen,
		VSPFee:               cfg.VSPFee,
		Network:              cfg.network,
		SupportEmail:         cfg.SupportEmail,
		VspClosed:            cfg.VspClosed,
		VspClosedMsg:         cfg.VspClosedMsg,
		AdminPass:            cfg.AdminPass,
		Debug:                cfg.WebServerDebug,
		Designation:          cfg.Designation,
		MaxVoteChangeRecords: maxVoteChangeRecords,
		VspdVersion:          version.String(),
	}
	api, err := webapi.New(db, cfg.logger("API"), dcrd, wallets, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialize webapi: %v", err)
		return 1
	}

	// WaitGroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup

	// Create a context that is canceled when a shutdown request is received
	// through an interrupt signal such as SIGINT (Ctrl+C).
	ctx := shutdownListener(log)

	// Start the webapi server.
	shutdownWg.Add(1)
	go func() {
		api.Run(ctx)
		shutdownWg.Done()
	}()

	// Start vspd.
	vspd := vspd.New(cfg.network, log, db, dcrd, wallets, blockNotifChan)
	shutdownWg.Add(1)
	go func() {
		vspd.Run(ctx)
		shutdownWg.Done()
	}()

	// Periodically write a database backup file.
	shutdownWg.Add(1)
	go func() {
		select {
		case <-ctx.Done():
			shutdownWg.Done()
			return
		case <-time.After(cfg.BackupInterval):
			err := db.WriteHotBackupFile()
			if err != nil {
				log.Errorf("Failed to write database backup: %v", err)
			}
		}
	}()

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return 0
}
