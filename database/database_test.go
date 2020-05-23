package database

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"
)

var (
	testDb = "test.db"
	db     *VspDatabase
)

func exampleTicket() Ticket {
	return Ticket{
		Hash:              "Hash",
		CommitmentAddress: "Address",
		FeeAddressIndex:   12345,
		FeeAddress:        "FeeAddress",
		SDiff:             1,
		BlockHeight:       2,
		VoteChoices:       map[string]string{"AgendaID": "Choice"},
		VotingKey:         "VotingKey",
		VSPFee:            0.1,
		FeeExpiration:     4,
		FeeTxHash:         "",
	}
}

// TestDatabase runs all database tests.
func TestDatabase(t *testing.T) {
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)

	// All sub-tests to run.
	tests := map[string]func(*testing.T){
		"testInsertTicket":       testInsertTicket,
		"testGetTicketByHash":    testGetTicketByHash,
		"testSetTicketVotingKey": testSetTicketVotingKey,
		"testUpdateExpireAndFee": testUpdateExpireAndFee,
		"testUpdateVoteChoices":  testUpdateVoteChoices,
	}

	for testName, test := range tests {
		// Create a new blank database for each sub-test.
		var err error
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.TODO())
		db, err = Open(ctx, &wg, testDb)
		if err != nil {
			t.Fatalf("error creating test database: %v", err)
		}

		// Run the sub-test.
		t.Run(testName, test)

		// Request database shutdown and wait for it to complete.
		cancel()
		wg.Wait()

		os.Remove(testDb)
	}
}

func testInsertTicket(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Inserting a ticket with the same hash should fail.
	err = db.InsertTicket(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with duplicate hash")
	}

	// Inserting a ticket with empty hash should fail.
	ticket.Hash = ""
	err = db.InsertTicket(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with no hash")
	}
}

func testGetTicketByHash(t *testing.T) {
	ticket := exampleTicket()
	// Insert a ticket into the database.
	err := db.InsertTicket(ticket)
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
		retrieved.SDiff != ticket.SDiff ||
		retrieved.BlockHeight != ticket.BlockHeight ||
		!reflect.DeepEqual(retrieved.VoteChoices, ticket.VoteChoices) ||
		retrieved.VotingKey != ticket.VotingKey ||
		retrieved.VSPFee != ticket.VSPFee ||
		retrieved.FeeTxHash != ticket.FeeTxHash ||
		retrieved.FeeExpiration != ticket.FeeExpiration {
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

func testSetTicketVotingKey(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Update values.
	newVotingKey := ticket.VotingKey + "2"
	newVoteChoices := ticket.VoteChoices
	feeTxHash := ticket.FeeTxHash + "3"
	newVoteChoices["AgendaID"] = "Different choice"
	err = db.SetTicketVotingKey(ticket.Hash, newVotingKey, newVoteChoices, feeTxHash)
	if err != nil {
		t.Fatalf("error updating votingkey and votechoices: %v", err)
	}

	// Retrieve ticket from database.
	retrieved, _, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}

	// Check ticket fields match expected.
	if !reflect.DeepEqual(newVoteChoices, retrieved.VoteChoices) ||
		feeTxHash != retrieved.FeeTxHash ||
		newVotingKey != retrieved.VotingKey {
		t.Fatal("retrieved ticket value didnt match expected")
	}
}

func testUpdateExpireAndFee(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Update ticket with new values.
	newExpiry := ticket.FeeExpiration + 1
	newFee := ticket.VSPFee + 1
	err = db.UpdateExpireAndFee(ticket.Hash, newExpiry, newFee)
	if err != nil {
		t.Fatalf("error updating expiry and fee: %v", err)
	}

	// Get updated ticket
	retrieved, _, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving updated ticket: %v", err)
	}

	// Check ticket fields match expected.
	if retrieved.VSPFee != newFee || retrieved.FeeExpiration != newExpiry {
		t.Fatal("retrieved ticket value didnt match expected")
	}
}

func testUpdateVoteChoices(t *testing.T) {
	// Insert a ticket into the database.
	ticket := exampleTicket()
	err := db.InsertTicket(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Update ticket with new votechoices.
	newVoteChoices := ticket.VoteChoices
	newVoteChoices["AgendaID"] = "Different choice"
	err = db.UpdateVoteChoices(ticket.Hash, newVoteChoices)
	if err != nil {
		t.Fatalf("error updating votechoices: %v", err)
	}

	// Get updated ticket
	retrieved, _, err := db.GetTicketByHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving updated ticket: %v", err)
	}

	// Check ticket fields match expected.
	if !reflect.DeepEqual(newVoteChoices, retrieved.VoteChoices) {
		t.Fatal("retrieved ticket value didnt match expected")
	}
}
