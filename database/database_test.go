package database

import (
	"context"
	"os"
	"sync"
	"testing"
)

var (
	testDb = "test.db"
	db     *VspDatabase
)

// TestDatabase runs all database tests.
func TestDatabase(t *testing.T) {
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)

	// All sub-tests to run.
	tests := map[string]func(*testing.T){
		"testInsertNewTicket": testInsertNewTicket,
		"testGetTicketByHash": testGetTicketByHash,
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

// TODO: Add tests for UpdateTicket, CountTickets, GetUnconfirmedTickets,
// GetPendingFees, GetUnconfirmedFees.

// TODO: Add tests for ticket.FeeExpired.
