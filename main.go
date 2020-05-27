package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jholdstock/dcrvsp/background"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
	"github.com/jholdstock/dcrvsp/webapi"
)

const (
	defaultFeeAddressExpiration = 1 * time.Hour
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

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	// Dial once just to validate config.
	dcrdConnect := rpc.Setup(ctx, &shutdownWg, cfg.DcrdUser, cfg.DcrdPass,
		cfg.DcrdHost, cfg.dcrdCert, nil)
	dcrdConn, err := dcrdConnect()
	if err != nil {
		log.Errorf("dcrd connection error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	_, err = rpc.DcrdClient(ctx, dcrdConn, cfg.netParams.Params)
	if err != nil {
		log.Errorf("dcrd client error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create RPC client for remote dcrwallet instance (used for voting).
	// Dial once just to validate config.
	walletConnect := rpc.Setup(ctx, &shutdownWg, cfg.WalletUser, cfg.WalletPass,
		cfg.WalletHost, cfg.walletCert, nil)
	walletConn, err := walletConnect()
	if err != nil {
		log.Errorf("dcrwallet connection error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	_, err = rpc.WalletClient(ctx, walletConn, cfg.netParams.Params)
	if err != nil {
		log.Errorf("dcrwallet client error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create a dcrd client with an attached notification handler which will run
	// in the background.
	notifHandler := &background.NotificationHandler{
		Ctx:           ctx,
		Db:            db,
		WalletConnect: walletConnect,
		NetParams:     cfg.netParams.Params,
	}
	dcrdWithNotifHandler := rpc.Setup(ctx, &shutdownWg, cfg.DcrdUser, cfg.DcrdPass,
		cfg.DcrdHost, cfg.dcrdCert, notifHandler)

	// Start background process which will continually attempt to reconnect to
	// dcrd if the connection drops.
	background.Start(notifHandler, dcrdWithNotifHandler)

	// Create and start webapi server.
	apiCfg := webapi.Config{
		VSPFee:               cfg.VSPFee,
		NetParams:            cfg.netParams.Params,
		FeeAddressExpiration: defaultFeeAddressExpiration,
	}
	err = webapi.Start(ctx, shutdownRequestChannel, &shutdownWg, cfg.Listen, db,
		dcrdConnect, walletConnect, cfg.WebServerDebug, cfg.FeeXPub, apiCfg)
	if err != nil {
		log.Errorf("Failed to initialize webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Wait for shutdown tasks to complete before returning.
	shutdownWg.Wait()

	return ctx.Err()
}
