// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

var (
	testDb               = "test.db"
	backupDb             = "test.db-backup"
	db                   *VspDatabase
	maxVoteChangeRecords = 3
)

// TestDatabase runs all database tests.
func TestDatabase(t *testing.T) {
	// Ensure we are starting with a clean environment.
	os.Remove(testDb)
	os.Remove(backupDb)

	// All sub-tests to run.
	tests := map[string]func(*testing.T){
		"testInsertNewTicket":   testInsertNewTicket,
		"testGetTicketByHash":   testGetTicketByHash,
		"testUpdateTicket":      testUpdateTicket,
		"testTicketFeeExpired":  testTicketFeeExpired,
		"testFilterTickets":     testFilterTickets,
		"testAddressIndex":      testAddressIndex,
		"testDeleteTicket":      testDeleteTicket,
		"testVoteChangeRecords": testVoteChangeRecords,
		"testHTTPBackup":        testHTTPBackup,
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
		db, err = Open(ctx, &wg, testDb, time.Hour, maxVoteChangeRecords)
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

func testHTTPBackup(t *testing.T) {
	// Capture the HTTP response written by the backup func.
	rr := httptest.NewRecorder()
	err := db.BackupDB(rr)
	if err != nil {
		t.Fatal(err)
	}

	// Check the HTTP status is OK.
	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("wrong HTTP status code: expected %v, got %v",
			http.StatusOK, status)
	}

	// Check HTTP headers.
	header := "Content-Type"
	expected := "application/octet-stream"
	if actual := rr.Header().Get(header); actual != expected {
		t.Errorf("wrong %s header: expected %s, got %s",
			header, expected, actual)
	}

	header = "Content-Disposition"
	expected = `attachment; filename="vspd.db"`
	if actual := rr.Header().Get(header); actual != expected {
		t.Errorf("wrong %s header: expected %s, got %s",
			header, expected, actual)
	}

	header = "Content-Length"
	cLength, err := strconv.Atoi(rr.Header().Get(header))
	if err != nil {
		t.Fatalf("could not convert %s to integer: %v", header, err)
	}

	if cLength <= 0 {
		t.Fatalf("expected a %s greater than zero, got %d", header, cLength)
	}

	// Check reported length matches actual.
	body, err := ioutil.ReadAll(rr.Result().Body)
	if err != nil {
		t.Fatalf("could not read http response body: %v", err)
	}

	if len(body) != cLength {
		t.Fatalf("expected reported content-length to match actual body length. %v != %v",
			cLength, len(body))
	}
}

// TODO: Add tests for CountTickets.
