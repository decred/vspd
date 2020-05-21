package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
	"github.com/jholdstock/dcrvsp/webapi"
)

const (
	feeAccountName = "fees"
	// TODO: Make expiration configurable?
	defaultFeeAddressExpiration = 24 * time.Hour
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

	// Create RPC client for local dcrwallet instance (used for generating fee
	// addresses and broadcasting fee transactions).
	feeWalletConnect := rpc.Setup(ctx, &shutdownWg, cfg.FeeWalletUser, cfg.FeeWalletPass, cfg.FeeWalletHost, cfg.feeWalletCert)
	feeWalletConn, err := feeWalletConnect()
	if err != nil {
		log.Errorf("Fee wallet connection error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	feeWalletClient, err := rpc.FeeWalletClient(ctx, feeWalletConn)
	if err != nil {
		log.Errorf("Fee wallet client error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create RPC client for remote dcrwallet instance (used for voting).
	votingWalletConnect := rpc.Setup(ctx, &shutdownWg, cfg.VotingWalletUser, cfg.VotingWalletPass, cfg.VotingWalletHost, cfg.votingWalletCert)
	votingWalletConn, err := votingWalletConnect()
	if err != nil {
		log.Errorf("Voting wallet connection error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	_, err = rpc.VotingWalletClient(ctx, votingWalletConn)
	if err != nil {
		log.Errorf("Voting wallet client error: %v", err)
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

	// Ensure the wallet account for collecting fees exists and matches config.
	err = setupFeeAccount(ctx, feeWalletClient, cfg.FeeXPub)
	if err != nil {
		log.Errorf("Fee account error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create and start webapi server.
	apiCfg := webapi.Config{
		SignKey:              signKey,
		PubKey:               pubKey,
		VSPFee:               cfg.VSPFee,
		NetParams:            cfg.netParams.Params,
		FeeAccountName:       feeAccountName,
		FeeAddressExpiration: defaultFeeAddressExpiration,
	}
	err = webapi.Start(ctx, shutdownRequestChannel, &shutdownWg, cfg.Listen, db, votingWalletConnect, feeWalletConnect, cfg.WebServerDebug, apiCfg)
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

func setupFeeAccount(ctx context.Context, walletClient *rpc.FeeWalletRPC, feeXpub string) error {
	// Check if account for fee collection already exists.
	accounts, err := walletClient.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("ListAccounts error: %v", err)
	}

	if _, ok := accounts[feeAccountName]; ok {
		// Account already exists. Check xpub matches xpub from config.
		existingXPub, err := walletClient.GetMasterPubKey(ctx, feeAccountName)
		if err != nil {
			return fmt.Errorf("GetMasterPubKey error: %v", err)
		}

		if existingXPub != feeXpub {
			return fmt.Errorf("existing account xpub differs from config: %s != %s", existingXPub, feeXpub)
		}

		log.Debugf("Using existing wallet account %q to collect fees", feeAccountName)

	} else {
		// Account does not exist. Create it using xpub from config.
		if err = walletClient.ImportXPub(ctx, feeAccountName, feeXpub); err != nil {
			log.Errorf("ImportXPub error: %v", err)
			return err
		}
		log.Debugf("Created new wallet account %q to collect fees", feeAccountName)
	}

	return nil
}
