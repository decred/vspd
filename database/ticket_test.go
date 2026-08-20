// Copyright (c) 2020-2026 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func exampleTicket() Ticket {
	return Ticket{
		Hash:              randString(64, hexCharset),
		CommitmentAddress: randString(35, addrCharset),
		FeeAddressIndex:   12345,
		FeeAddressXPubID:  10,
		FeeAddress:        randString(35, addrCharset),
		FeeAmount:         1e7,
		FeeExpiration:     4,
		Confirmed:         false,
		VoteChoices:       map[string]string{"AgendaID": "yes"},
		TSpendPolicy:      map[string]string{randString(64, hexCharset): "no"},
		TreasuryPolicy:    map[string]string{randString(66, hexCharset): "abstain"},
		VotingWIF:         randString(53, addrCharset),
		FeeTxHex:          randString(504, hexCharset),
		FeeTxHash:         randString(64, hexCharset),
		FeeTxStatus:       FeeBroadcast,
	}
}

func testInsertNewTicket(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Insert another ticket.
	err = db.InsertNewTicket(exampleTicket())
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Inserting a ticket with the same hash should fail.
	ticket2 := exampleTicket()
	ticket2.Hash = ticket.Hash
	err = db.InsertNewTicket(ticket2)
	if err == nil {
		t.Fatal("expected an error inserting ticket with duplicate hash")
	}

	// Inserting a ticket with empty hash should fail.
	ticket.Hash = ""
	err = db.InsertNewTicket(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with no hash")
	}
}

func testDeleteTicket(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Delete ticket.
	err = db.DeleteTicket(ticket)
	if err != nil {
		t.Fatalf("error deleting ticket: %v", err)
	}

	// Nothing should be in the db.
	_, found, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}
	if found {
		t.Fatal("expected found==false")
	}
}

func testGetTicketByHash(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Retrieve ticket from database.
	retrieved, found, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}
	if !found {
		t.Fatal("expected found==true")
	}

	// Check ticket fields match expected.
	if !reflect.DeepEqual(retrieved, ticket) {
		t.Fatal("retrieved ticket value didnt match expected")
	}

	// Check found==false when requesting a non-existent ticket.
	_, found, err = db.GetTicketByHash("Not a real ticket hash")
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}
	if found {
		t.Fatal("expected found==false")
	}
}

func testUpdateTicket(t *testing.T) {
	ticket := exampleTicket()
	// Insert a ticket into the database.
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Update ticket with new values.
	ticket.FeeAmount = ticket.FeeAmount + 1
	ticket.FeeExpiration = ticket.FeeExpiration + 1
	ticket.VoteChoices = map[string]string{"New agenda": "New value"}
	ticket.FeeAddressXPubID = 20

	err = db.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("error updating ticket: %v", err)
	}

	// Retrieve updated ticket from database.
	retrieved, found, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}
	if !found {
		t.Fatal("expected found==true")
	}

	if !reflect.DeepEqual(retrieved, ticket) {
		t.Fatal("retrieved ticket value didnt match expected")
	}

	// Updating a non-existent ticket should fail.
	ticket.Hash = "doesnt exist"
	err = db.UpdateTicket(ticket)
	if err == nil {
		t.Fatal("expected an error updating a ticket with non-existent hash")
	}
}

func testTicketFeeExpired(t *testing.T) {
	ticket := exampleTicket()

	now := time.Now()
	hourBefore := now.Add(-time.Hour).Unix()
	hourAfter := now.Add(time.Hour).Unix()

	ticket.FeeExpiration = hourAfter
	if ticket.FeeExpired() {
		t.Fatal("expected ticket not to be expired")
	}

	ticket.FeeExpiration = hourBefore
	if !ticket.FeeExpired() {
		t.Fatal("expected ticket to be expired")
	}
}

func testFilterTickets(t *testing.T) {
	// Insert a ticket.
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Insert another ticket.
	ticket2 := exampleTicket()
	ticket2.Confirmed = !ticket.Confirmed
	err = db.InsertNewTicket(ticket2)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Expect all tickets returned.
	retrieved, err := db.filterTickets(func(_ *bolt.Bucket) bool {
		return true
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 2 {
		t.Fatalf("expected to find 2 tickets, found %d", len(retrieved))
	}

	// Only one ticket should be confirmed.
	retrieved, err = db.filterTickets(func(t *bolt.Bucket) bool {
		return bytesToBool(t.Get(confirmedK))
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 1 {
		t.Fatalf("expected to find 1 ticket, found %d", len(retrieved))
	}
	if retrieved[0].Confirmed != true {
		t.Fatal("expected retrieved ticket to be confirmed")
	}

	// Expect no tickets with confirmed fee.
	retrieved, err = db.filterTickets(func(t *bolt.Bucket) bool {
		return FeeStatus(t.Get(feeTxStatusK)) == FeeConfirmed
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 0 {
		t.Fatalf("expected to find 0 tickets, found %d", len(retrieved))
	}
}

func testTicketStatsCounts(t *testing.T) {
	count := func(test string, expectedVoting, expectedVoted, expectedExpired, expectedMissed int64) {
		t.Helper()
		stats, err := db.TicketStats(69420)
		if err != nil {
			t.Fatalf("error getting ticket stats: %v", err)
		}

		if stats.Voting != expectedVoting {
			t.Fatalf("test %s: expected %d voting tickets, got %d",
				test, expectedVoting, stats.Voting)
		}
		if stats.Voted != expectedVoted {
			t.Fatalf("test %s: expected %d voted tickets, got %d",
				test, expectedVoted, stats.Voted)
		}
		if stats.Expired != expectedExpired {
			t.Fatalf("test %s: expected %d expired tickets, got %d",
				test, expectedExpired, stats.Expired)
		}
		if stats.Missed != expectedMissed {
			t.Fatalf("test %s: expected %d missed tickets, got %d",
				test, expectedMissed, stats.Missed)
		}
	}

	// Initial counts should all be zero.
	count("empty db", 0, 0, 0, 0)

	// Insert a ticket with non-confirmed fee into the database.
	// This should not be counted.
	ticket := exampleTicket()
	ticket.FeeTxStatus = FeeReceieved
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("unconfirmed fee", 0, 0, 0, 0)

	// Insert a ticket with confirmed fee into the database.
	// This should be counted.
	ticket2 := exampleTicket()
	ticket2.FeeTxStatus = FeeConfirmed
	err = db.InsertNewTicket(ticket2)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("confirmed fee", 1, 0, 0, 0)

	// Insert a voted ticket into the database.
	// This should be counted.
	ticket3 := exampleTicket()
	ticket3.FeeTxStatus = FeeConfirmed
	ticket3.Outcome = Voted
	err = db.InsertNewTicket(ticket3)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("voted", 1, 1, 0, 0)

	// Insert an expired ticket into the database.
	// This should be counted.
	ticket4 := exampleTicket()
	ticket4.FeeTxStatus = FeeConfirmed
	ticket4.Outcome = Expired
	err = db.InsertNewTicket(ticket4)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("expired", 1, 1, 1, 0)

	// Insert a missed ticket into the database.
	// This should be counted.
	ticket5 := exampleTicket()
	ticket5.FeeTxStatus = FeeConfirmed
	ticket5.Outcome = Missed
	err = db.InsertNewTicket(ticket5)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("missed", 1, 1, 1, 1)

	// Insert a revoked ticket into the database.
	// This should be counted as expired.
	ticket6 := exampleTicket()
	ticket6.FeeTxStatus = FeeConfirmed
	ticket6.Outcome = Revoked
	err = db.InsertNewTicket(ticket6)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	count("revoked", 1, 1, 2, 1)
}

func testTicketStatsRevenue(t *testing.T) {
	const blockHeight = 69420

	const (
		within24Hours = blockHeight - 1
		within28Days  = blockHeight - blocksIn24Hours - 1
		before28Days  = blockHeight - blocksIn28Days - 1
	)

	checkRevenue := func(test string, expectedLifetime, expected28Days, expected24Hours int64) {
		t.Helper()
		stats, err := db.TicketStats(blockHeight)
		if err != nil {
			t.Fatalf("error getting ticket stats: %v", err)
		}

		if stats.RevenueLifetime != expectedLifetime {
			t.Fatalf("test %s: expected lifetime revenue of %d, got %d",
				test, expectedLifetime, stats.RevenueLifetime)
		}
		if stats.Revenue28Days != expected28Days {
			t.Fatalf("test %s: expected 28 day revenue of %d, got %d",
				test, expected28Days, stats.Revenue28Days)
		}
		if stats.Revenue24Hours != expected24Hours {
			t.Fatalf("test %s: expected 24 hour revenue of %d, got %d",
				test, expected24Hours, stats.Revenue24Hours)
		}
	}

	insertTicket := func(status FeeStatus, purchaseHeight int64) {
		t.Helper()
		ticket := exampleTicket()
		ticket.FeeTxStatus = status
		ticket.PurchaseHeight = purchaseHeight

		err := db.InsertNewTicket(ticket)
		if err != nil {
			t.Fatalf("error storing ticket in database: %v", err)
		}
	}

	// An empty database has earned no revenue.
	checkRevenue("empty db", 0, 0, 0)

	// A ticket without a confirmed fee tx has not earned any revenue.
	insertTicket(NoFee, within24Hours)
	checkRevenue("no fee", 0, 0, 0)
	insertTicket(FeeReceieved, within24Hours)
	checkRevenue("received fee", 0, 0, 0)
	insertTicket(FeeBroadcast, within24Hours)
	checkRevenue("broadcast fee", 0, 0, 0)
	insertTicket(FeeError, within24Hours)
	checkRevenue("errored fee", 0, 0, 0)

	// A fee confirmed within the last 24 hours contributes to every period.
	insertTicket(FeeConfirmed, within24Hours)
	checkRevenue("within 24 hours", 1e7, 1e7, 1e7)

	// A fee confirmed within the last 28 days does not contribute to the 24
	// hour period.
	insertTicket(FeeConfirmed, within28Days)
	checkRevenue("within 28 days", 2e7, 2e7, 1e7)

	// A fee confirmed before both periods only contributes to lifetime
	// revenue.
	insertTicket(FeeConfirmed, before28Days)
	checkRevenue("outside periods", 3e7, 2e7, 1e7)

	// A ticket which is not yet confirmed on-chain has no purchase height. It's
	// only just been purchased, so it's fee contributes to every period.
	insertTicket(FeeConfirmed, 0)
	checkRevenue("no purchase height", 4e7, 3e7, 2e7)
}
