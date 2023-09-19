// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package vspd

import (
	"context"
	"strings"
)

// checkWalletConsistency will retrieve all votable tickets from the database
// and ensure they are all added to voting wallets with the correct vote
// choices.
func (v *Vspd) checkWalletConsistency(ctx context.Context) {
	const funcName = "checkWalletConsistency"

	v.log.Debug("Checking voting wallet consistency")

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
		// Exit early if context has been canceled.
		if ctx.Err() != nil {
			return
		}

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

			v.log.Debugf("Adding missing ticket (wallet=%s, ticketHash=%s)",
				walletClient.String(), dbTicket.Hash)

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
			v.log.Infof("Performing a rescan on wallet %s (fromHeight=%d)",
				walletClient.String(), minHeight)
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
		// Exit early if context has been canceled.
		if ctx.Err() != nil {
			return
		}

		// Get all tickets the wallet is aware of.
		walletTickets, err := walletClient.TicketInfo(oldestHeight)
		if err != nil {
			v.log.Errorf("%s: dcrwallet.TicketInfo failed (startHeight=%d, wallet=%s): %v",
				funcName, oldestHeight, walletClient.String(), err)
			continue
		}

		for _, dbTicket := range votableTickets {
			// Exit early if context has been canceled.
			if ctx.Err() != nil {
				return
			}

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

				v.log.Debugf("Updating incorrect consensus vote choices (wallet=%s, agenda=%s, ticketHash=%s)",
					walletClient.String(), dbAgenda, dbTicket.Hash)

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
