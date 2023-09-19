// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package vspd

import (
	"context"
	"fmt"

	"github.com/decred/vspd/database"
)

// checkDatabaseIntegrity starts the process of ensuring that all data expected
// to be in the database is present and up to date.
func (v *Vspd) checkDatabaseIntegrity(ctx context.Context) error {
	err := v.checkPurchaseHeights()
	if err != nil {
		return fmt.Errorf("checkPurchaseHeights error: %w", err)
	}

	err = v.checkRevoked(ctx)
	if err != nil {
		return fmt.Errorf("checkRevoked error: %w", err)
	}

	return nil
}

// checkPurchaseHeights ensures a purchase height is recorded for all confirmed
// tickets in the database. This is necessary because of an old bug which, in
// some circumstances, would prevent purchase height from being stored.
func (v *Vspd) checkPurchaseHeights() error {
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
func (v *Vspd) checkRevoked(ctx context.Context) error {
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
	startHeight := revoked.EarliestPurchaseHeight() + int64(v.network.TicketMaturity)

	spent, _, err := v.findSpentTickets(ctx, revoked, startHeight)
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
