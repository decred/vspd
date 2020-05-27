package database

import (
	"reflect"
	"testing"
)

func exampleTicket() Ticket {
	return Ticket{
		Hash:              "Hash",
		CommitmentAddress: "Address",
		FeeAddressIndex:   12345,
		FeeAddress:        "FeeAddress",
		FeeAmount:         0.1,
		FeeExpiration:     4,
		Confirmed:         false,
		VoteChoices:       map[string]string{"AgendaID": "Choice"},
		VotingWIF:         "VotingKey",
		FeeTxHex:          "FeeTransction",
		FeeTxHash:         "",
		FeeConfirmed:      true,
	}
}

func testInsertNewTicket(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertNewTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Inserting a ticket with the same hash should fail.
	err = db.InsertNewTicket(ticket)
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
		retrieved.FeeConfirmed != ticket.FeeConfirmed {
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
