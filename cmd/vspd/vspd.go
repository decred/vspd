// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/version"
	"github.com/decred/vspd/webapi"
	"github.com/jrick/wsrpc/v2"
)

const (
	// requiredConfs is the number of confirmations required to consider a
	// ticket purchase or a fee transaction to be final.
	requiredConfs = 6

	// maxVoteChangeRecords defines how many vote change records will be stored
	// for each ticket. The limit is in place to mitigate DoS attacks on server
	// storage space. When storing a new record breaches this limit, the oldest
	// record in the database is deleted.
	maxVoteChangeRecords = 10

	// consistencyInterval is the time period between wallet consistency checks.
	consistencyInterval = 30 * time.Minute

	// dcrdInterval is the time period between dcrd connection checks.
	dcrdInterval = time.Second * 15
)

type vspd struct {
	cfg     *config
	log     slog.Logger
	db      *database.VspDatabase
	dcrd    rpc.DcrdConnect
	wallets rpc.WalletConnect

	blockNotifChan chan *wire.BlockHeader

	// lastScannedBlock is the height of the most recent block which has been
	// scanned for spent tickets.
	lastScannedBlock int64
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

	// Create a channel to receive blockConnected notifications from dcrd.
	blockNotifChan := make(chan *wire.BlockHeader)

	// Create RPC client for local dcrd instance (used for broadcasting and
	// checking the status of fee transactions).
	dcrd := rpc.SetupDcrd(cfg.DcrdUser, cfg.DcrdPass, cfg.DcrdHost, cfg.dcrdCert, cfg.netParams.Params, rpcLog, blockNotifChan)

	// Create RPC client for remote dcrwallet instances (used for voting).
	wallets := rpc.SetupWallet(cfg.walletUsers, cfg.walletPasswords, cfg.walletHosts, cfg.walletCerts, cfg.netParams.Params, rpcLog)

	v := &vspd{
		cfg:     cfg,
		log:     log,
		db:      db,
		dcrd:    dcrd,
		wallets: wallets,

		blockNotifChan: blockNotifChan,
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
	ctx := shutdownListener(v.log)

	// Run database integrity checks to ensure all data in database is present
	// and up-to-date.
	err := v.checkDatabaseIntegrity()
	if err != nil {
		// vspd should still start if this fails, so just log an error.
		v.log.Errorf("Database integrity check failed: %v", err)
	}

	// Run the block connected handler now to catch up with any blocks mined
	// while vspd was shut down.
	v.blockConnected()

	// Run voting wallet consistency check now to ensure all wallets are up to
	// date.
	v.checkWalletConsistency()

	// WaitGroup for services to signal when they have shutdown cleanly.
	var shutdownWg sync.WaitGroup

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
	err = webapi.Start(ctx, requestShutdown, &shutdownWg, v.cfg.Listen, v.db, v.cfg.logger("API"),
		v.dcrd, v.wallets, apiCfg)
	if err != nil {
		v.log.Errorf("Failed to initialize webapi: %v", err)
		requestShutdown()
		shutdownWg.Wait()
		return 1
	}

	// Start all background tasks and notification handlers.
	shutdownWg.Add(1)
	go func() {
		backupTicker := time.NewTicker(v.cfg.BackupInterval)
		consistencyTicker := time.NewTicker(consistencyInterval)
		dcrdTicker := time.NewTicker(dcrdInterval)

		for {
			select {

			// Periodically write a database backup file.
			case <-backupTicker.C:
				err := v.db.WriteHotBackupFile()
				if err != nil {
					v.log.Errorf("Failed to write database backup: %v", err)
				}

			// Run voting wallet consistency check periodically.
			case <-consistencyTicker.C:
				v.checkWalletConsistency()

			// Ensure dcrd client is connected so notifications are received.
			case <-dcrdTicker.C:
				_, _, err := v.dcrd.Client()
				if err != nil {
					v.log.Errorf("dcrd connect error: %v", err)
				}

			// Handle blockconnected notifications from dcrd.
			case header := <-v.blockNotifChan:
				v.log.Debugf("Block notification %d (%s)", header.Height, header.BlockHash().String())
				v.blockConnected()

			// Handle shutdown request.
			case <-ctx.Done():
				backupTicker.Stop()
				consistencyTicker.Stop()
				dcrdTicker.Stop()
				shutdownWg.Done()
				return
			}
		}
	}()

	// Wait for shutdown tasks to complete before running deferred tasks and
	// returning.
	shutdownWg.Wait()

	return 0
}

// checkDatabaseIntegrity starts the process of ensuring that all data expected
// to be in the database is present and up to date.
func (v *vspd) checkDatabaseIntegrity() error {
	err := v.checkPurchaseHeights()
	if err != nil {
		return fmt.Errorf("checkPurchaseHeights error: %w", err)
	}

	err = v.checkRevoked()
	if err != nil {
		return fmt.Errorf("checkRevoked error: %w", err)
	}

	return nil
}

// checkPurchaseHeights ensures a purchase height is recorded for all confirmed
// tickets in the database. This is necessary because of an old bug which, in
// some circumstances, would prevent purchase height from being stored.
func (v *vspd) checkPurchaseHeights() error {
	missing, err := v.db.GetMissingPurchaseHeight()
	if err != nil {
		// Cannot proceed if this fails, return.
		return fmt.Errorf("db.GetMissingPurchaseHeight error: %w", err)
	}

	if len(missing) == 0 {
		// Nothing to do, return.
		return nil
	}

	v.log.Warnf("%d tickets are missing purchase heights", len(missing))

	dcrdClient, _, err := v.dcrd.Client()
	if err != nil {
		// Cannot proceed if this fails, return.
		return err
	}

	fixed := 0
	for _, ticket := range missing {
		tktTx, err := dcrdClient.GetRawTransaction(ticket.Hash)
		if err != nil {
			// Just log and continue, other tickets might succeed.
			v.log.Errorf("Could not get raw tx for ticket %s: %v", ticket.Hash, err)
			continue
		}
		ticket.PurchaseHeight = tktTx.BlockHeight
		err = v.db.UpdateTicket(ticket)
		if err != nil {
			// Just log and continue, other tickets might succeed.
			v.log.Errorf("Could not insert purchase height for ticket %s: %v", ticket.Hash, err)
			continue
		}
		fixed++
	}

	v.log.Infof("Added missing purchase height to %d tickets", fixed)
	return nil
}

// checkRevoked ensures that any tickets in the database with outcome set to
// revoked are updated to either expired or missed.
func (v *vspd) checkRevoked() error {
	revoked, err := v.db.GetRevokedTickets()
	if err != nil {
		return fmt.Errorf("db.GetRevoked error: %w", err)
	}

	if len(revoked) == 0 {
		// Nothing to do, return.
		return nil
	}

	v.log.Warnf("Updating %s in revoked status, this may take a while...",
		pluralize(len(revoked), "ticket"))

	// Search for the transactions which spend these tickets, starting at the
	// earliest height one of them matured.
	startHeight := revoked.EarliestPurchaseHeight() + int64(v.cfg.netParams.TicketMaturity)

	spent, _, err := v.findSpentTickets(revoked, startHeight)
	if err != nil {
		return fmt.Errorf("findSpentTickets error: %w", err)
	}

	fixedMissed := 0
	fixedExpired := 0

	// Update database with correct voted status.
	for hash, spentTicket := range spent {
		switch {
		case spentTicket.voted():
			v.log.Errorf("Ticket voted but was recorded as revoked. Please contact "+
				"developers so this can be investigated (ticketHash=%s)", hash)
			continue
		case spentTicket.missed():
			spentTicket.dbTicket.Outcome = database.Missed
			fixedMissed++
		default:
			spentTicket.dbTicket.Outcome = database.Expired
			fixedExpired++
		}

		err = v.db.UpdateTicket(spentTicket.dbTicket)
		if err != nil {
			v.log.Errorf("Could not update status of ticket %s: %v", hash, err)
		}
	}

	v.log.Infof("%s updated (%d missed, %d expired)",
		pluralize(fixedExpired+fixedMissed, "revoked ticket"),
		fixedMissed, fixedExpired)

	return nil
}

// blockConnected is called once when vspd starts up, and once each time a
// blockconnected notification is received from dcrd.
func (v *vspd) blockConnected() {
	const funcName = "blockConnected"

	dcrdClient, _, err := v.dcrd.Client()
	if err != nil {
		v.log.Errorf("%s: %v", funcName, err)
		return
	}

	// Step 1/4: Update the database with any tickets which now have 6+
	// confirmations.

	unconfirmed, err := v.db.GetUnconfirmedTickets()
	if err != nil {
		v.log.Errorf("%s: db.GetUnconfirmedTickets error: %v", funcName, err)
	}

	for _, ticket := range unconfirmed {
		tktTx, err := dcrdClient.GetRawTransaction(ticket.Hash)
		if err != nil {
			// ErrNoTxInfo here probably indicates a tx which was never mined
			// and has been removed from the mempool. For example, a ticket
			// purchase tx close to an sdiff change, or a ticket purchase tx
			// which expired. Remove it from the db.
			var e *wsrpc.Error
			if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
				v.log.Infof("%s: Removing unconfirmed ticket from db - no information available "+
					"about transaction (ticketHash=%s)", funcName, ticket.Hash)

				err = v.db.DeleteTicket(ticket)
				if err != nil {
					v.log.Errorf("%s: db.DeleteTicket error (ticketHash=%s): %v",
						funcName, ticket.Hash, err)
				}

				// This will not error if an alternate signing address does not
				// exist for ticket.
				err = v.db.DeleteAltSignAddr(ticket.Hash)
				if err != nil {
					v.log.Errorf("%s: db.DeleteAltSignAddr error (ticketHash=%s): %v",
						funcName, ticket.Hash, err)
				}
			} else {
				v.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
			}

			continue
		}

		if tktTx.Confirmations >= requiredConfs {
			ticket.PurchaseHeight = tktTx.BlockHeight
			ticket.Confirmed = true
			err = v.db.UpdateTicket(ticket)
			if err != nil {
				v.log.Errorf("%s: db.UpdateTicket error, failed to set ticket as confirmed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}

			v.log.Infof("%s: Ticket confirmed (ticketHash=%s)", funcName, ticket.Hash)
		}
	}

	// Step 2/4: Broadcast fee tx for tickets which are confirmed.

	pending, err := v.db.GetPendingFees()
	if err != nil {
		v.log.Errorf("%s: db.GetPendingFees error: %v", funcName, err)
	}

	for _, ticket := range pending {
		err = dcrdClient.SendRawTransaction(ticket.FeeTxHex)
		if err != nil {
			v.log.Errorf("%s: dcrd.SendRawTransaction for fee tx failed (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
			ticket.FeeTxStatus = database.FeeError
		} else {
			v.log.Infof("%s: Fee tx broadcast for ticket (ticketHash=%s, feeHash=%s)",
				funcName, ticket.Hash, ticket.FeeTxHash)
			ticket.FeeTxStatus = database.FeeBroadcast
		}

		err = v.db.UpdateTicket(ticket)
		if err != nil {
			v.log.Errorf("%s: db.UpdateTicket error, failed to set fee tx as broadcast (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
		}
	}

	// Step 3/4: Add tickets with confirmed fees to voting wallets.

	unconfirmedFees, err := v.db.GetUnconfirmedFees()
	if err != nil {
		v.log.Errorf("%s: db.GetUnconfirmedFees error: %v", funcName, err)
	}

	walletClients, failedConnections := v.wallets.Clients()
	if len(walletClients) == 0 {
		v.log.Errorf("%s: Could not connect to any wallets", funcName)
		return
	}
	if len(failedConnections) > 0 {
		v.log.Errorf("%s: Failed to connect to %d wallet(s), proceeding with only %d",
			funcName, len(failedConnections), len(walletClients))
	}

	for _, ticket := range unconfirmedFees {
		feeTx, err := dcrdClient.GetRawTransaction(ticket.FeeTxHash)
		if err != nil {
			v.log.Errorf("%s: dcrd.GetRawTransaction for fee tx failed (feeTxHash=%s, ticketHash=%s): %v",
				funcName, ticket.FeeTxHash, ticket.Hash, err)

			ticket.FeeTxStatus = database.FeeError
			err = v.db.UpdateTicket(ticket)
			if err != nil {
				v.log.Errorf("%s: db.UpdateTicket error, failed to set fee tx status to error (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
			}
			continue
		}

		// If fee is confirmed, update the database and add ticket to voting
		// wallets.
		if feeTx.Confirmations >= requiredConfs {
			// We no longer need the hex once the tx is confirmed on-chain.
			ticket.FeeTxHex = ""
			ticket.FeeTxStatus = database.FeeConfirmed
			err = v.db.UpdateTicket(ticket)
			if err != nil {
				v.log.Errorf("%s: db.UpdateTicket error, failed to set fee tx as confirmed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}
			v.log.Infof("%s: Fee tx confirmed (ticketHash=%s)", funcName, ticket.Hash)

			// Add ticket to the voting wallet.

			rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
			if err != nil {
				v.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}
			for _, walletClient := range walletClients {
				err = walletClient.AddTicketForVoting(ticket.VotingWIF, rawTicket.BlockHash, rawTicket.Hex)
				if err != nil {
					v.log.Errorf("%s: dcrwallet.AddTicketForVoting error (wallet=%s, ticketHash=%s): %v",
						funcName, walletClient.String(), ticket.Hash, err)
					continue
				}

				// Set consensus vote choices on voting wallets.
				for agenda, choice := range ticket.VoteChoices {
					err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
					if err != nil {
						if strings.Contains(err.Error(), "no agenda with ID") {
							v.log.Warnf("%s: Removing invalid agenda from ticket vote choices (ticketHash=%s, agenda=%s)",
								funcName, ticket.Hash, agenda)
							delete(ticket.VoteChoices, agenda)
							err = v.db.UpdateTicket(ticket)
							if err != nil {
								v.log.Errorf("%s: db.UpdateTicket error, failed to remove invalid agenda (ticketHash=%s): %v",
									funcName, ticket.Hash, err)
							}
						} else {
							v.log.Errorf("%s: dcrwallet.SetVoteChoice error (wallet=%s, ticketHash=%s): %v",
								funcName, walletClient.String(), ticket.Hash, err)
						}
					}
				}

				// Set tspend policy on voting wallets.
				for tspend, policy := range ticket.TSpendPolicy {
					err = walletClient.SetTSpendPolicy(tspend, policy, ticket.Hash)
					if err != nil {
						v.log.Errorf("%s: dcrwallet.SetTSpendPolicy failed (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), ticket.Hash, err)
					}
				}

				// Set treasury policy on voting wallets.
				for key, policy := range ticket.TreasuryPolicy {
					err = walletClient.SetTreasuryPolicy(key, policy, ticket.Hash)
					if err != nil {
						v.log.Errorf("%s: dcrwallet.SetTreasuryPolicy failed (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), ticket.Hash, err)
					}
				}

				v.log.Infof("%s: Ticket added to voting wallet (wallet=%s, ticketHash=%s)",
					funcName, walletClient.String(), ticket.Hash)
			}
		}
	}

	// Step 4/4: Set ticket outcome in database if any tickets are voted/revoked.

	votableTickets, err := v.db.GetVotableTickets()
	if err != nil {
		v.log.Errorf("%s: db.GetVotableTickets failed: %v", funcName, err)
		return
	}

	// If the database has no votable tickets, there is nothing more to do.
	if len(votableTickets) == 0 {
		return
	}

	var startHeight int64
	if v.lastScannedBlock == 0 {
		// Use the earliest height at which a votable ticket matured if vspd has
		// not performed a scan for spent tickets since it started. This will
		// catch any tickets which were spent whilst vspd was offline.
		startHeight = votableTickets.EarliestPurchaseHeight() + int64(v.cfg.netParams.TicketMaturity)
	} else {
		startHeight = v.lastScannedBlock
	}

	spent, endHeight, err := v.findSpentTickets(votableTickets, startHeight)
	if err != nil {
		v.log.Errorf("%s: findSpentTickets error: %v", funcName, err)
		return
	}

	v.lastScannedBlock = endHeight

	for _, spentTicket := range spent {
		dbTicket := spentTicket.dbTicket

		switch {
		case spentTicket.voted():
			dbTicket.Outcome = database.Voted
		case spentTicket.missed():
			dbTicket.Outcome = database.Missed
		default:
			dbTicket.Outcome = database.Expired
		}

		err = v.db.UpdateTicket(dbTicket)
		if err != nil {
			v.log.Errorf("%s: db.UpdateTicket error, failed to set ticket outcome (ticketHash=%s): %v",
				funcName, dbTicket.Hash, err)
			continue
		}

		v.log.Infof("%s: Ticket %s %s at height %d", funcName,
			dbTicket.Hash, dbTicket.Outcome, spentTicket.heightSpent)
	}
}

// checkWalletConsistency will retrieve all votable tickets from the database
// and ensure they are all added to voting wallets with the correct vote
// choices.
func (v *vspd) checkWalletConsistency() {
	const funcName = "checkWalletConsistency"

	v.log.Info("Checking voting wallet consistency")

	dcrdClient, _, err := v.dcrd.Client()
	if err != nil {
		v.log.Errorf("%s: %v", funcName, err)
		return
	}

	walletClients, failedConnections := v.wallets.Clients()
	if len(walletClients) == 0 {
		v.log.Errorf("%s: Could not connect to any wallets", funcName)
		return
	}
	if len(failedConnections) > 0 {
		v.log.Errorf("%s: Failed to connect to %d wallet(s), proceeding with only %d",
			funcName, len(failedConnections), len(walletClients))
	}

	// Step 1/2: Check all tickets are added to all voting wallets.

	votableTickets, err := v.db.GetVotableTickets()
	if err != nil {
		v.log.Errorf("%s: db.GetVotableTickets failed: %v", funcName, err)
		return
	}

	// If the database has no votable tickets, there is nothing more to do
	if len(votableTickets) == 0 {
		return
	}

	// Find the oldest block height from confirmed tickets.
	oldestHeight := votableTickets.EarliestPurchaseHeight()

	// Iterate over each wallet and add any missing tickets.
	for _, walletClient := range walletClients {
		// Get all tickets the wallet is aware of.
		walletTickets, err := walletClient.TicketInfo(oldestHeight)
		if err != nil {
			v.log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
				funcName, oldestHeight, walletClient.String(), err)
			continue
		}

		// If missing tickets are added, set a flag and keep track of the
		// earliest purchase height.
		var added bool
		var minHeight int64
		for _, dbTicket := range votableTickets {
			// If wallet already knows this ticket, skip to the next one.
			_, exists := walletTickets[dbTicket.Hash]
			if exists {
				continue
			}

			v.log.Debugf("%s: Adding missing ticket (wallet=%s, ticketHash=%s)",
				funcName, walletClient.String(), dbTicket.Hash)

			rawTicket, err := dcrdClient.GetRawTransaction(dbTicket.Hash)
			if err != nil {
				v.log.Errorf("%s: dcrd.GetRawTransaction error: %v", funcName, err)
				continue
			}

			err = walletClient.AddTicketForVoting(dbTicket.VotingWIF, rawTicket.BlockHash, rawTicket.Hex)
			if err != nil {
				v.log.Errorf("%s: dcrwallet.AddTicketForVoting error (wallet=%s, ticketHash=%s): %v",
					funcName, walletClient.String(), dbTicket.Hash, err)
				continue
			}

			added = true
			if minHeight == 0 || minHeight > rawTicket.BlockHeight {
				minHeight = rawTicket.BlockHeight
			}
		}

		// Perform a rescan if any missing tickets were added to this wallet.
		if added {
			v.log.Infof("%s: Performing a rescan on wallet %s (fromHeight=%d)",
				funcName, walletClient.String(), minHeight)
			err = walletClient.RescanFrom(minHeight)
			if err != nil {
				v.log.Errorf("%s: dcrwallet.RescanFrom failed (wallet=%s): %v",
					funcName, walletClient.String(), err)
				continue
			}
		}
	}

	// Step 2/2: Ensure vote choices are set correctly for all tickets on
	// all wallets.

	for _, walletClient := range walletClients {
		// Get all tickets the wallet is aware of.
		walletTickets, err := walletClient.TicketInfo(oldestHeight)
		if err != nil {
			v.log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
				funcName, oldestHeight, walletClient.String(), err)
			continue
		}

		for _, dbTicket := range votableTickets {
			// All tickets should be added to all wallets at this point, so log
			// a warning if any are still missing.
			walletTicket, exists := walletTickets[dbTicket.Hash]
			if !exists {
				v.log.Warnf("%s: Ticket missing from voting wallet (wallet=%s, ticketHash=%s)",
					funcName, walletClient.String(), dbTicket.Hash)
				continue
			}

			// Check if consensus vote choices match
			for dbAgenda, dbChoice := range dbTicket.VoteChoices {
				match := false
				for _, walletChoice := range walletTicket.Choices {
					if walletChoice.AgendaID == dbAgenda && walletChoice.ChoiceID == dbChoice {
						match = true
					}
				}

				// Skip to next agenda if db and wallet are matching.
				if match {
					continue
				}

				v.log.Debugf("%s: Updating incorrect consensus vote choices (wallet=%s, agenda=%s, ticketHash=%s)",
					funcName, walletClient.String(), dbAgenda, dbTicket.Hash)

				// If db and wallet are not matching, update wallet with correct
				// choice.
				err = walletClient.SetVoteChoice(dbAgenda, dbChoice, dbTicket.Hash)
				if err != nil {
					if strings.Contains(err.Error(), "no agenda with ID") {
						v.log.Warnf("%s: Removing invalid agenda from ticket vote choices (ticketHash=%s, agenda=%s)",
							funcName, dbTicket.Hash, dbAgenda)
						delete(dbTicket.VoteChoices, dbAgenda)
						err = v.db.UpdateTicket(dbTicket)
						if err != nil {
							v.log.Errorf("%s: db.UpdateTicket error, failed to remove invalid agenda (ticketHash=%s): %v",
								funcName, dbTicket.Hash, err)
						}
					} else {
						v.log.Errorf("%s: dcrwallet.SetVoteChoice error (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), dbTicket.Hash, err)
					}
				}
			}

			// TODO - tspend and treasury policy consistency checking.
		}
	}
}
