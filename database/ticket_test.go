// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
	"time"
)

func exampleTicket() Ticket {
	return Ticket{
		Hash:              "Hash",
		CommitmentAddress: "Address",
		FeeAddressIndex:   12345,
		FeeAddress:        "FeeAddress",
		FeeAmount:         10000000,
		FeeExpiration:     4,
		Confirmed:         false,
		VoteChoices:       map[string]string{"AgendaID": "Choice"},
		VotingWIF:         "VotingKey",
		FeeTxHex:          "FeeTransction",
		FeeTxHash:         "",
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

	// Inserting a ticket with different fee address but same hash should fail.
	ticket2 := exampleTicket()
	ticket2.FeeAddress = ticket.FeeAddress + "2"
	err = db.InsertNewTicket(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with duplicate hash")
	}

	// Inserting a ticket with different hash but same fee address should fail.
	ticket3 := exampleTicket()
	ticket3.FeeAddress = ticket.Hash + "2"
	err = db.InsertNewTicket(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with duplicate fee addr")
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

	// Delete ticket
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
	ticket := exampleTicket()
	// Insert a ticket into the database.
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
	if retrieved.Hash != ticket.Hash ||
		retrieved.CommitmentAddress != ticket.CommitmentAddress ||
		retrieved.FeeAddressIndex != ticket.FeeAddressIndex ||
		retrieved.FeeAddress != ticket.FeeAddress ||
		retrieved.FeeAmount != ticket.FeeAmount ||
		retrieved.FeeExpiration != ticket.FeeExpiration ||
		retrieved.Confirmed != ticket.Confirmed ||
		!reflect.DeepEqual(retrieved.VoteChoices, ticket.VoteChoices) ||
		retrieved.VotingWIF != ticket.VotingWIF ||
		retrieved.FeeTxHex != ticket.FeeTxHex ||
		retrieved.FeeTxHash != ticket.FeeTxHash ||
		retrieved.FeeTxStatus != ticket.FeeTxStatus {
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

	// Update ticket with new values
	ticket.FeeAmount = ticket.FeeAmount + 1
	ticket.FeeExpiration = ticket.FeeExpiration + 1
	err = db.UpdateTicket(ticket)
	if err != nil {
		t.Fatalf("error updating ticket: %v", err)
	}

	// Retrieve ticket from database.
	retrieved, found, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}
	if !found {
		t.Fatal("expected found==true")
	}

	if ticket.FeeAmount != retrieved.FeeAmount ||
		ticket.FeeExpiration != retrieved.FeeExpiration {
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
	// Insert a ticket
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Insert another ticket
	ticket.Hash = ticket.Hash + "1"
	ticket.FeeAddress = ticket.FeeAddress + "1"
	ticket.FeeAddressIndex = ticket.FeeAddressIndex + 1
	ticket.Confirmed = !ticket.Confirmed
	err = db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Expect all tickets returned.
	retrieved, err := db.filterTickets(func(t Ticket) bool {
		return true
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 2 {
		t.Fatal("expected to find 2 tickets")
	}

	// Only one ticket should be confirmed.
	retrieved, err = db.filterTickets(func(t Ticket) bool {
		return t.Confirmed
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 1 {
		t.Fatal("expected to find 2 tickets")
	}
	if retrieved[0].Confirmed != true {
		t.Fatal("expected retrieved ticket to be confirmed")
	}

	// Expect no tickets with confirmed fee.
	retrieved, err = db.filterTickets(func(t Ticket) bool {
		return t.FeeTxStatus == FeeConfirmed
	})
	if err != nil {
		t.Fatalf("error filtering tickets: %v", err)
	}
	if len(retrieved) != 0 {
		t.Fatal("expected to find 0 tickets")
	}
}
