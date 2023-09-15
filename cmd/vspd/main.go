// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/decred/dcrd/wire"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/internal/config"
	"github.com/decred/vspd/internal/version"
	"github.com/decred/vspd/rpc"
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

	vspd := newVspd(cfg, log, db, dcrd, wallets, blockNotifChan)

	// Create a context that is canceled when a shutdown request is received
	// through an interrupt signal.
	ctx := shutdownListener(log)

	return vspd.run(ctx)
}
