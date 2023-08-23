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
	"github.com/decred/slog"
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
	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadConfig error: %v\n", err)
		os.Exit(1)
	}

	vspd, err := newVspd(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "newVspd error: %v\n", err)
		os.Exit(1)
	}

	// Run until an exit code is returned.
	os.Exit(vspd.run())
}

type vspd struct {
	cfg     *config
	log     slog.Logger
	db      *database.VspDatabase
	dcrd    rpc.DcrdConnect
	wallets rpc.WalletConnect
}

// newVspd creates the essential resources required by vspd - a database, logger
// and RPC clients - then returns an instance of vspd which is ready to be run.
func newVspd(cfg *config) (*vspd, error) {
	// Open database.
	db, err := database.Open(cfg.dbPath, cfg.logger(" DB"), maxVoteChangeRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	log := cfg.logger("VSP")
	rpcLog := cfg.logger("RPC")

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, cfg.netParams.Params, rpcLog)

	// Create RPC client for remote dcrwallet instances (used for voting).
	wallets := rpc.SetupWallet(cfg.walletUsers, cfg.walletPasswords, cfg.walletHosts, cfg.walletCerts, cfg.netParams.Params, rpcLog)

	v := &vspd{
		cfg:     cfg,
		log:     log,
		db:      db,
		dcrd:    dcrd,
		wallets: wallets,
	}

	return v, nil
}

// run starts all of vspds background services including the web server, and
// stops all started services when a shutdown is requested.
func (v *vspd) run() int {
	v.log.Criticalf("Version %s (Go version %s %s/%s)", version.String(), runtime.Version(),
		runtime.GOOS, runtime.GOARCH)

	if v.cfg.netParams == &mainNetParams &&
		version.PreRelease != "" {
		v.log.Warnf("")
		v.log.Warnf("\tWARNING: This is a pre-release version of vspd which should not be used on mainnet.")
		v.log.Warnf("")
	}

	if v.cfg.VspClosed {
		v.log.Warnf("")
		v.log.Warnf("\tWARNING: Config --vspclosed is set. This will prevent vspd from accepting new tickets.")
		v.log.Warnf("")
	}

	// Defer shutdown tasks.
	defer v.log.Criticalf("Shutdown complete")
	const writeBackup = true
	defer v.db.Close(writeBackup)
	defer v.dcrd.Close()
	defer v.wallets.Close()

	// Create a context that is cancelled when a shutdown request is received
	// through an interrupt signal.
	shutdownCtx := shutdownListener(v.log)

	// WaitGroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup

	v.db.WritePeriodicBackups(shutdownCtx, &shutdownWg, v.cfg.BackupInterval)

	// Ensure all data in database is present and up-to-date.
	err := v.db.CheckIntegrity(v.dcrd)
	if err != nil {
		// vspd should still start if this fails, so just log an error.
		v.log.Errorf("Could not check database integrity: %v", err)
	}

	// Run the block connected handler now to catch up with any blocks mined
	// while vspd was shut down.
	v.blockConnected()

	// Run voting wallet consistency check now to ensure all wallets are up to
	// date.
	v.checkWalletConsistency()

	// Run voting wallet consistency check periodically.
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-shutdownCtx.Done():
				shutdownWg.Done()
				return
			case <-time.After(consistencyInterval):
				v.checkWalletConsistency()
			}
		}
	}()

	// Create and start webapi server.
	apiCfg := webapi.Config{
		VSPFee:               v.cfg.VSPFee,
		NetParams:            v.cfg.netParams.Params,
		BlockExplorerURL:     v.cfg.netParams.blockExplorerURL,
		SupportEmail:         v.cfg.SupportEmail,
		VspClosed:            v.cfg.VspClosed,
		VspClosedMsg:         v.cfg.VspClosedMsg,
		AdminPass:            v.cfg.AdminPass,
		Debug:                v.cfg.WebServerDebug,
		Designation:          v.cfg.Designation,
		MaxVoteChangeRecords: maxVoteChangeRecords,
		VspdVersion:          version.String(),
	}
	err = webapi.Start(shutdownCtx, requestShutdown, &shutdownWg, v.cfg.Listen, v.db, v.cfg.logger("API"),
		v.dcrd, v.wallets, apiCfg)
	if err != nil {
		v.log.Errorf("Failed to initialize webapi: %v", err)
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
				v.log.Debugf("Block notification %d (%s)", header.Height, header.BlockHash().String())
				v.blockConnected()
			}
		}
	}()

	// Attach notification listener to dcrd client.
	v.dcrd.BlockConnectedHandler(notifChan)

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
				_, _, err := v.dcrd.Client()
				if err != nil {
					v.log.Errorf("dcrd connect error: %v", err)
				}
			}
		}
	}()

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return 0
}
