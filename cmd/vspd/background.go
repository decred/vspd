// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"strings"

	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/jrick/wsrpc/v2"
)

const (
	// requiredConfs is the number of confirmations required to consider a
	// ticket purchase or a fee transaction to be final.
	requiredConfs = 6
)

// blockConnected is called once when vspd starts up, and once each time a
// blockconnected notification is received from dcrd.
func blockConnected(dcrdRPC rpc.DcrdConnect, walletRPC rpc.WalletConnect, db *database.VspDatabase, log slog.Logger) {

	const funcName = "blockConnected"

	dcrdClient, _, err := dcrdRPC.Client()
	if err != nil {
		log.Errorf("%s: %v", funcName, err)
		return
	}

	// Step 1/4: Update the database with any tickets which now have 6+
	// confirmations.

	unconfirmed, err := db.GetUnconfirmedTickets()
	if err != nil {
		log.Errorf("%s: db.GetUnconfirmedTickets error: %v", funcName, err)
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
				log.Infof("%s: Removing unconfirmed ticket from db - no information available "+
					"about transaction (ticketHash=%s)", funcName, ticket.Hash)

				err = db.DeleteTicket(ticket)
				if err != nil {
					log.Errorf("%s: db.DeleteTicket error (ticketHash=%s): %v",
						funcName, ticket.Hash, err)
				}

				// This will not error if an alternate signing address does not
				// exist for ticket.
				err = db.DeleteAltSignAddr(ticket.Hash)
				if err != nil {
					log.Errorf("%s: db.DeleteAltSignAddr error (ticketHash=%s): %v",
						funcName, ticket.Hash, err)
				}
			} else {
				log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
			}

			continue
		}

		if tktTx.Confirmations >= requiredConfs {
			ticket.PurchaseHeight = tktTx.BlockHeight
			ticket.Confirmed = true
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to set ticket as confirmed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}

			log.Infof("%s: Ticket confirmed (ticketHash=%s)", funcName, ticket.Hash)
		}
	}

	// Step 2/4: Broadcast fee tx for tickets which are confirmed.

	pending, err := db.GetPendingFees()
	if err != nil {
		log.Errorf("%s: db.GetPendingFees error: %v", funcName, err)
	}

	for _, ticket := range pending {
		err = dcrdClient.SendRawTransaction(ticket.FeeTxHex)
		if err != nil {
			log.Errorf("%s: dcrd.SendRawTransaction for fee tx failed (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
			ticket.FeeTxStatus = database.FeeError
		} else {
			log.Infof("%s: Fee tx broadcast for ticket (ticketHash=%s, feeHash=%s)",
				funcName, ticket.Hash, ticket.FeeTxHash)
			ticket.FeeTxStatus = database.FeeBroadcast
		}

		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("%s: db.UpdateTicket error, failed to set fee tx as broadcast (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
		}
	}

	// Step 3/4: Add tickets with confirmed fees to voting wallets.

	unconfirmedFees, err := db.GetUnconfirmedFees()
	if err != nil {
		log.Errorf("%s: db.GetUnconfirmedFees error: %v", funcName, err)
	}

	walletClients, failedConnections := walletRPC.Clients()
	if len(walletClients) == 0 {
		log.Errorf("%s: Could not connect to any wallets", funcName)
		return
	}
	if len(failedConnections) > 0 {
		log.Errorf("%s: Failed to connect to %d wallet(s), proceeding with only %d",
			funcName, len(failedConnections), len(walletClients))
	}

	for _, ticket := range unconfirmedFees {
		feeTx, err := dcrdClient.GetRawTransaction(ticket.FeeTxHash)
		if err != nil {
			log.Errorf("%s: dcrd.GetRawTransaction for fee tx failed (feeTxHash=%s, ticketHash=%s): %v",
				funcName, ticket.FeeTxHash, ticket.Hash, err)

			ticket.FeeTxStatus = database.FeeError
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to set fee tx status to error (ticketHash=%s): %v",
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
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to set fee tx as confirmed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}
			log.Infof("%s: Fee tx confirmed (ticketHash=%s)", funcName, ticket.Hash)

			// Add ticket to the voting wallet.

			rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
			if err != nil {
				log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}
			for _, walletClient := range walletClients {
				err = walletClient.AddTicketForVoting(ticket.VotingWIF, rawTicket.BlockHash, rawTicket.Hex)
				if err != nil {
					log.Errorf("%s: dcrwallet.AddTicketForVoting error (wallet=%s, ticketHash=%s): %v",
						funcName, walletClient.String(), ticket.Hash, err)
					continue
				}

				// Set consensus vote choices on voting wallets.
				for agenda, choice := range ticket.VoteChoices {
					err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
					if err != nil {
						if strings.Contains(err.Error(), "no agenda with ID") {
							log.Warnf("%s: Removing invalid agenda from ticket vote choices (ticketHash=%s, agenda=%s)",
								funcName, ticket.Hash, agenda)
							delete(ticket.VoteChoices, agenda)
							err = db.UpdateTicket(ticket)
							if err != nil {
								log.Errorf("%s: db.UpdateTicket error, failed to remove invalid agenda (ticketHash=%s): %v",
									funcName, ticket.Hash, err)
							}
						} else {
							log.Errorf("%s: dcrwallet.SetVoteChoice error (wallet=%s, ticketHash=%s): %v",
								funcName, walletClient.String(), ticket.Hash, err)
						}
					}
				}

				// Set tspend policy on voting wallets.
				for tspend, policy := range ticket.TSpendPolicy {
					err = walletClient.SetTSpendPolicy(tspend, policy, ticket.Hash)
					if err != nil {
						log.Errorf("%s: dcrwallet.SetTSpendPolicy failed (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), ticket.Hash, err)
					}
				}

				// Set treasury policy on voting wallets.
				for key, policy := range ticket.TreasuryPolicy {
					err = walletClient.SetTreasuryPolicy(key, policy, ticket.Hash)
					if err != nil {
						log.Errorf("%s: dcrwallet.SetTreasuryPolicy failed (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), ticket.Hash, err)
					}
				}

				log.Infof("%s: Ticket added to voting wallet (wallet=%s, ticketHash=%s)",
					funcName, walletClient.String(), ticket.Hash)
			}
		}
	}

	// Step 4/4: Set ticket outcome in database if any tickets are voted/revoked.

	// Ticket status needs to be checked on every wallet. This is because only
	// one of the voting wallets will actually succeed in voting/revoking
	// tickets (the others will get errors like "tx already exists"). Only the
	// successful wallet will have the most up-to-date ticket status, the others
	// will be outdated.
	for _, walletClient := range walletClients {
		votableTickets, err := db.GetVotableTickets()
		if err != nil {
			log.Errorf("%s: db.GetVotableTickets failed: %v", funcName, err)
			continue
		}

		// If the database has no votable tickets, there is nothing more to do
		if len(votableTickets) == 0 {
			break
		}

		// Find the oldest block height from confirmed tickets.
		oldestHeight := votableTickets.EarliestPurchaseHeight()

		ticketInfo, err := walletClient.TicketInfo(oldestHeight)
		if err != nil {
			log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
				funcName, oldestHeight, walletClient.String(), err)
			continue
		}

		for _, dbTicket := range votableTickets {
			tInfo, ok := ticketInfo[dbTicket.Hash]
			if !ok {
				log.Warnf("%s: TicketInfo response did not include expected ticket (wallet=%s, ticketHash=%s)",
					funcName, walletClient.String(), dbTicket.Hash)
				continue
			}

			switch tInfo.Status {
			case "missed", "expired", "revoked":
				dbTicket.Outcome = database.Revoked
			case "voted":
				dbTicket.Outcome = database.Voted
			default:
				// Skip to next ticket.
				continue
			}

			err = db.UpdateTicket(dbTicket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to set ticket outcome (ticketHash=%s): %v",
					funcName, dbTicket.Hash, err)
				continue
			}

			log.Infof("%s: Ticket no longer votable: outcome=%s, ticketHash=%s", funcName,
				dbTicket.Outcome, dbTicket.Hash)
		}
	}

}

// checkWalletConsistency will retrieve all votable tickets from the database
// and ensure they are all added to voting wallets with the correct vote
// choices.
func checkWalletConsistency(dcrdRPC rpc.DcrdConnect, walletRPC rpc.WalletConnect, db *database.VspDatabase, log slog.Logger) {

	const funcName = "checkWalletConsistency"

	log.Info("Checking voting wallet consistency")

	dcrdClient, _, err := dcrdRPC.Client()
	if err != nil {
		log.Errorf("%s: %v", funcName, err)
		return
	}

	walletClients, failedConnections := walletRPC.Clients()
	if len(walletClients) == 0 {
		log.Errorf("%s: Could not connect to any wallets", funcName)
		return
	}
	if len(failedConnections) > 0 {
		log.Errorf("%s: Failed to connect to %d wallet(s), proceeding with only %d",
			funcName, len(failedConnections), len(walletClients))
	}

	// Step 1/2: Check all tickets are added to all voting wallets.

	votableTickets, err := db.GetVotableTickets()
	if err != nil {
		log.Errorf("%s: db.GetVotableTickets failed: %v", funcName, err)
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
			log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
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

			log.Debugf("%s: Adding missing ticket (wallet=%s, ticketHash=%s)",
				funcName, walletClient.String(), dbTicket.Hash)

			rawTicket, err := dcrdClient.GetRawTransaction(dbTicket.Hash)
			if err != nil {
				log.Errorf("%s: dcrd.GetRawTransaction error: %v", funcName, err)
				continue
			}

			err = walletClient.AddTicketForVoting(dbTicket.VotingWIF, rawTicket.BlockHash, rawTicket.Hex)
			if err != nil {
				log.Errorf("%s: dcrwallet.AddTicketForVoting error (wallet=%s, ticketHash=%s): %v",
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
			log.Infof("%s: Performing a rescan on wallet %s (fromHeight=%d)",
				funcName, walletClient.String(), minHeight)
			err = walletClient.RescanFrom(minHeight)
			if err != nil {
				log.Errorf("%s: dcrwallet.RescanFrom failed (wallet=%s): %v",
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
			log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
				funcName, oldestHeight, walletClient.String(), err)
			continue
		}

		for _, dbTicket := range votableTickets {
			// All tickets should be added to all wallets at this point, so log
			// a warning if any are still missing.
			walletTicket, exists := walletTickets[dbTicket.Hash]
			if !exists {
				log.Warnf("%s: Ticket missing from voting wallet (wallet=%s, ticketHash=%s)",
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

				log.Debugf("%s: Updating incorrect consensus vote choices (wallet=%s, agenda=%s, ticketHash=%s)",
					funcName, walletClient.String(), dbAgenda, dbTicket.Hash)

				// If db and wallet are not matching, update wallet with correct
				// choice.
				err = walletClient.SetVoteChoice(dbAgenda, dbChoice, dbTicket.Hash)
				if err != nil {
					if strings.Contains(err.Error(), "no agenda with ID") {
						log.Warnf("%s: Removing invalid agenda from ticket vote choices (ticketHash=%s, agenda=%s)",
							funcName, dbTicket.Hash, dbAgenda)
						delete(dbTicket.VoteChoices, dbAgenda)
						err = db.UpdateTicket(dbTicket)
						if err != nil {
							log.Errorf("%s: db.UpdateTicket error, failed to remove invalid agenda (ticketHash=%s): %v",
								funcName, dbTicket.Hash, err)
						}
					} else {
						log.Errorf("%s: dcrwallet.SetVoteChoice error (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), dbTicket.Hash, err)
					}
				}
			}

			// TODO - tspend and treasury policy consistency checking.
		}
	}
}
