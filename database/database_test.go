// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

var (
	testDb   = "test.db"
	backupDb = "test.db-backup"
	db       *VspDatabase
)

// TestDatabase runs all database tests.
func TestDatabase(t *testing.T) {
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)
	os.Remove(backupDb)

	// All sub-tests to run.
	tests := map[string]func(*testing.T){
		"testInsertNewTicket":  testInsertNewTicket,
		"testGetTicketByHash":  testGetTicketByHash,
		"testUpdateTicket":     testUpdateTicket,
		"testTicketFeeExpired": testTicketFeeExpired,
		"testFilterTickets":    testFilterTickets,
		"testAddressIndex":     testAddressIndex,
		"testDeleteTicket":     testDeleteTicket,
	}

	for testName, test := range tests {
		// Create a new blank database for each sub-test.
		var err error
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.TODO())
		err = CreateNew(testDb, "feexpub")
		if err != nil {
			t.Fatalf("error creating test database: %v", err)
		}
		db, err = Open(ctx, &wg, testDb, time.Hour)
		if err != nil {
			t.Fatalf("error opening test database: %v", err)
		}

		// Run the sub-test.
		t.Run(testName, test)

		// Request database shutdown and wait for it to complete.
		cancel()
		wg.Wait()

		os.Remove(testDb)
		os.Remove(backupDb)
	}
}

// TODO: Add tests for CountTickets.
