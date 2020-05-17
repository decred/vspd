package database

import (
	"context"
	"os"
	"sync"
	"testing"
)

var (
	testDb = "test.db"
	ticket = Ticket{
		Hash:                "Hash",
		CommitmentAddress:   "Address",
		CommitmentSignature: "CommitmentSignature",
		FeeAddress:          "FeeAddress",
		SDiff:               1,
		BlockHeight:         2,
		VoteBits:            3,
		VotingKey:           "VotingKey",
		Expiration:          4,
	}
	db *VspDatabase
)

// TestDatabase runs all database tests.
func TestDatabase(t *testing.T) {
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)

	// All sub-tests to run.
	tests := map[string]func(*testing.T){
		"testInsertFeeAddress":          testInsertFeeAddress,
		"testGetFeeAddressByTicketHash": testGetFeeAddressByTicketHash,
		"testGetFeesByFeeAddress":       testGetFeesByFeeAddress,
		"testInsertFeeAddressVotingKey": testInsertFeeAddressVotingKey,
		"testGetInactiveFeeAddresses":   testGetInactiveFeeAddresses,
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

func testInsertFeeAddress(t *testing.T) {
	// Insert a ticket into the database.
	err := db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Inserting a ticket with the same hash should fail.
	err = db.InsertFeeAddress(ticket)
	if err == nil {
		t.Fatal("expected an error inserting ticket with duplicate hash")
	}
}

func testGetFeeAddressByTicketHash(t *testing.T) {
	// Insert a ticket into the database.
	err := db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Retrieve ticket from database.
	retrieved, err := db.GetFeeAddressByTicketHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}

	// Check ticket fields match expected.
	if retrieved.Hash != ticket.Hash ||
		retrieved.CommitmentAddress != ticket.CommitmentAddress ||
		retrieved.CommitmentSignature != ticket.CommitmentSignature ||
		retrieved.FeeAddress != ticket.FeeAddress ||
		retrieved.SDiff != ticket.SDiff ||
		retrieved.BlockHeight != ticket.BlockHeight ||
		retrieved.VoteBits != ticket.VoteBits ||
		retrieved.VotingKey != ticket.VotingKey ||
		retrieved.Expiration != ticket.Expiration {
		t.Fatal("retrieved ticket value didnt match expected")
	}

	// Error if non-existent ticket requested.
	_, err = db.GetFeeAddressByTicketHash("Not a real ticket hash")
	if err == nil {
		t.Fatal("expected an error while retrieving a non-existent ticket")
	}
}

func testGetFeesByFeeAddress(t *testing.T) {
	// Insert a ticket into the database.
	err := db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Retrieve ticket using its fee address.
	retrieved, err := db.GetFeesByFeeAddress(ticket.FeeAddress)
	if err != nil {
		t.Fatalf("error retrieving ticket by fee address: %v", err)
	}

	// Check it is the correct ticket.
	if retrieved.FeeAddress != ticket.FeeAddress {
		t.Fatal("retrieved ticket FeeAddress didnt match expected")
	}

	// Error if non-existent ticket requested.
	_, err = db.GetFeesByFeeAddress("Not a real fee address")
	if err == nil {
		t.Fatal("expected an error while retrieving a non-existent ticket")
	}

	// Insert another ticket into the database with the same fee address.
	ticket.Hash = ticket.Hash + "2"
	err = db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Error when more than one ticket matches
	_, err = db.GetFeesByFeeAddress(ticket.FeeAddress)
	if err == nil {
		t.Fatal("expected an error when multiple tickets are found")
	}
}

func testInsertFeeAddressVotingKey(t *testing.T) {
	// Insert a ticket into the database.
	err := db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Update values.
	newVotingKey := ticket.VotingKey + "2"
	newVoteBits := ticket.VoteBits + 2
	err = db.InsertFeeAddressVotingKey(ticket.CommitmentAddress, newVotingKey, newVoteBits)
	if err != nil {
		t.Fatalf("error updating votingkey and votebits: %v", err)
	}

	// Retrieve ticket from database.
	retrieved, err := db.GetFeeAddressByTicketHash(ticket.Hash)
	if err != nil {
		t.Fatalf("error retrieving ticket by ticket hash: %v", err)
	}

	// Check ticket fields match expected.
	if newVoteBits != retrieved.VoteBits ||
		newVotingKey != retrieved.VotingKey {
		t.Fatal("retrieved ticket value didnt match expected")
	}
}

func testGetInactiveFeeAddresses(t *testing.T) {
	// Insert a ticket into the database.
	err := db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Insert a ticket with empty voting key into the database.
	ticket.Hash = ticket.Hash + "2"
	newFeeAddr := ticket.FeeAddress + "2"
	ticket.FeeAddress = newFeeAddr
	ticket.VotingKey = ""
	err = db.InsertFeeAddress(ticket)
	if err != nil {
		t.Fatalf("error storing ticket in database: %v", err)
	}

	// Retrieve unused fee address from database.
	feeAddrs, err := db.GetInactiveFeeAddresses()
	if err != nil {
		t.Fatalf("error retrieving inactive fee addresses: %v", err)
	}

	// Check we have one value, and its the expected one.
	if len(feeAddrs) != 1 {
		t.Fatal("expected 1 unused fee address")
	}
	if feeAddrs[0] != newFeeAddr {
		t.Fatal("fee address didnt match expected")
	}
}
