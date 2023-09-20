// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
)

func exampleRecord() VoteChangeRecord {
	return VoteChangeRecord{
		Request:           "Request",
		RequestSignature:  "RequestSignature",
		Response:          "Response",
		ResponseSignature: "ResponseSignature",
	}
}

func testVoteChangeRecords(t *testing.T) {
	const hash = "MyHash"
	record := exampleRecord()

	// Insert a record into the database.
	err := db.SaveVoteChange(hash, record)
	if err != nil {
		t.Fatalf("error storing vote change record in database: %v", err)
	}

	// Retrieve record and check values.
	retrieved, err := db.GetVoteChanges(hash)
	if err != nil {
		t.Fatalf("error retrieving vote change records: %v", err)
	}

	if len(retrieved) != 1 || !reflect.DeepEqual(retrieved[0], record) {
		t.Fatal("retrieved record didnt match expected")
	}

	// Insert some more records, giving us one greater than the limit.
	for i := 0; i < maxVoteChangeRecords; i++ {
		err = db.SaveVoteChange(hash, record)
		if err != nil {
			t.Fatalf("error storing vote change record in database: %v", err)
		}
	}

	// Retrieve records.
	retrieved, err = db.GetVoteChanges(hash)
	if err != nil {
		t.Fatalf("error retrieving vote change records: %v", err)
	}

	// Oldest record should have been deleted.
	if len(retrieved) != maxVoteChangeRecords {
		t.Fatalf("vote change record limit breached")
	}

	if _, ok := retrieved[0]; ok {
		t.Fatalf("oldest vote change record should have been deleted")
	}
}
