package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jholdstock/vspd/background"
	"github.com/jholdstock/vspd/database"
	"github.com/jholdstock/vspd/rpc"
	"github.com/jholdstock/vspd/webapi"
)

const (
	defaultFeeAddressExpiration = 1 * time.Hour
	defaultConsistencyInterval  = 10 * time.Second
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

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	// Dial once just to validate config.
	// TODO: Failed RPC connection should not prevent vspd starting up.
	dcrdConnect := rpc.Setup(ctx, &shutdownWg, cfg.DcrdUser, cfg.DcrdPass,
		cfg.DcrdHost, cfg.dcrdCert, nil)
	dcrdConn, err := dcrdConnect()
	if err != nil {
		log.Errorf("dcrd connection error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}
	dcrdClient, err := rpc.DcrdClient(ctx, dcrdConn, cfg.netParams.Params)
	if err != nil {
		log.Errorf("dcrd client error: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return err
	}

	// Create RPC client for remote dcrwallet instance (used for voting).
	// Dial once just to validate config.
	// TODO: Failed RPC connection should not prevent vspd starting up.
	walletConnect := make([]rpc.Connect, len(cfg.WalletHosts))
	walletConn := make([]rpc.Caller, len(cfg.WalletHosts))
	walletClient := make([]*rpc.WalletRPC, len(cfg.WalletHosts))

	for i := 0; i < len(cfg.WalletHosts); i++ {
		walletConnect[i] = rpc.Setup(ctx, &shutdownWg, cfg.WalletUser, cfg.WalletPass,
			cfg.WalletHosts[i], cfg.walletCert, nil)
		walletConn[i], err = walletConnect[i]()
		if err != nil {
			log.Errorf("dcrwallet connection error: %v", err)
			requestShutdown()
			shutdownWg.Wait()
			return err
		}
		walletClient[i], err = rpc.WalletClient(ctx, walletConn[i], cfg.netParams.Params)
		if err != nil {
			log.Errorf("dcrwallet client error: %v", err)
			requestShutdown()
			shutdownWg.Wait()
			return err
		}
	}

	// TODO: This needs to move into background.go eventually.
	go func() {
		ticker := time.NewTicker(defaultConsistencyInterval)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:

				// TODO: This gets all tickets with confirmed fees regardless of
				// status. It should not get voted/missed/expired tickets.
				dbTickets, err := db.GetConfirmedFees()
				if err != nil {
					log.Errorf("GetConfirmedFees failed: %v", err)
					continue
				}

				log.Debugf("Checking voting wallets - should have %d tickets", len(dbTickets))

				for i := 0; i < len(walletClient); i++ {
					walletTickets, err := walletClient[i].GetTickets()
					if err != nil {
						log.Errorf("GetTickets failed: %v", err)
						continue
					}

					log.Infof("Wallet %s has %d tickets", walletClient[i].String(), len(walletTickets))

					// Add any missing private keys and tickets to wallet.
					for _, dbTicket := range dbTickets {
						_, exists := walletTickets[dbTicket.Hash]
						if exists {
							continue
						}

						log.Infof("Adding missing ticket hash %s", dbTicket.Hash)

						// TODO: Need to reconnect wallet client here incase it has errored.
						err = walletClient[i].ImportPrivKey(dbTicket.VotingWIF)
						if err != nil {
							log.Errorf("importprivkey failed: %v", err)
							continue
						}

						rawTicket, err := dcrdClient.GetRawTransaction(dbTicket.Hash)
						if err != nil {
							log.Errorf("GetRawTransaction error: %v", err)
							continue
						}

						err = walletClient[i].AddTransaction(rawTicket.BlockHash, rawTicket.Hex)
						if err != nil {
							log.Errorf("AddTransaction error: %v", err)
							continue
						}

						// TODO: Set voting preferences.
					}

				}
			}
		}
	}()

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
		SupportEmail:         cfg.SupportEmail,
		VspClosed:            cfg.VspClosed,
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
