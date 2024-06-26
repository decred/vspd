// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"testing"
)

func testFeeXPub(t *testing.T) {
	// A newly created DB should store the fee xpub it was initialized with.
	retrievedXPub, err := db.FeeXPub()
	if err != nil {
		t.Fatalf("error getting fee xpub: %v", err)
	}

	if retrievedXPub.Key != feeXPub {
		t.Fatalf("expected fee xpub %v, got %v", feeXPub, retrievedXPub.Key)
	}

	// The ID, last used index and retirement timestamp should all be 0
	if retrievedXPub.ID != 0 {
		t.Fatalf("expected xpub ID 0, got %d", retrievedXPub.ID)
	}
	if retrievedXPub.LastUsedIdx != 0 {
		t.Fatalf("expected xpub last used 0, got %d", retrievedXPub.LastUsedIdx)
	}
	if retrievedXPub.Retired != 0 {
		t.Fatalf("expected xpub retirement 0, got %d", retrievedXPub.Retired)
	}

	// Update address index.
	idx := uint32(99)
	err = db.SetLastAddressIndex(idx)
	if err != nil {
		t.Fatalf("error setting address index: %v", err)
	}

	// Check for updated value.
	retrievedXPub, err = db.FeeXPub()
	if err != nil {
		t.Fatalf("error getting fee xpub: %v", err)
	}
	if retrievedXPub.LastUsedIdx != idx {
		t.Fatalf("expected xpub last used %d, got %d", idx, retrievedXPub.LastUsedIdx)
	}

	// Key, ID and retirement timestamp should be unchanged.
	if retrievedXPub.Key != feeXPub {
		t.Fatalf("expected fee xpub %v, got %v", feeXPub, retrievedXPub.Key)
	}
	if retrievedXPub.ID != 0 {
		t.Fatalf("expected xpub ID 0, got %d", retrievedXPub.ID)
	}
	if retrievedXPub.Retired != 0 {
		t.Fatalf("expected xpub retirement 0, got %d", retrievedXPub.Retired)
	}
}

func testRetireFeeXPub(t *testing.T) {
	// Increment the last used index to simulate some usage.
	idx := uint32(99)
	err := db.SetLastAddressIndex(idx)
	if err != nil {
		t.Fatalf("error setting address index: %v", err)
	}

	// Ensure a previously used xpub is rejected.
	err = db.RetireXPub(feeXPub)
	if err == nil {
		t.Fatalf("previous xpub was not rejected")
	}

	const expectedErr = "provided xpub has already been used"
	if err == nil || err.Error() != expectedErr {
		t.Fatalf("incorrect error, expected %q, got %q",
			expectedErr, err.Error())
	}

	// An unused xpub should be accepted.
	const feeXPub2 = "feexpub2"
	err = db.RetireXPub(feeXPub2)
	if err != nil {
		t.Fatalf("retiring xpub failed: %v", err)
	}

	// Retrieve the new xpub. Index should be incremented, last addr should be
	// reset to 0, key should not be retired.
	retrievedXPub, err := db.FeeXPub()
	if err != nil {
		t.Fatalf("error getting fee xpub: %v", err)
	}

	if retrievedXPub.Key != feeXPub2 {
		t.Fatalf("expected fee xpub %q, got %q", feeXPub2, retrievedXPub.Key)
	}
	if retrievedXPub.ID != 1 {
		t.Fatalf("expected xpub ID 1, got %d", retrievedXPub.ID)
	}
	if retrievedXPub.LastUsedIdx != 0 {
		t.Fatalf("expected xpub last used 0, got %d", retrievedXPub.LastUsedIdx)
	}
	if retrievedXPub.Retired != 0 {
		t.Fatalf("expected xpub retirement 0, got %d", retrievedXPub.Retired)
	}

	// Old xpub should have retired field set.
	xpubs, err := db.AllXPubs()
	if err != nil {
		t.Fatalf("error getting all fee xpubs: %v", err)
	}

	if xpubs[0].Retired == 0 {
		t.Fatalf("old xpub retired field not set")
	}
}
