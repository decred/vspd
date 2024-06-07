// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"testing"
)

func testFeeXPub(t *testing.T) {
	// A newly created DB should store the fee xpub it was initialized with, and
	// the last used index should be 0.
	retrievedXPub, err := db.FeeXPub()
	if err != nil {
		t.Fatalf("error getting fee xpub: %v", err)
	}

	if retrievedXPub.Key != feeXPub {
		t.Fatalf("expected fee xpub %v, got %v", feeXPub, retrievedXPub.Key)
	}

	if retrievedXPub.LastUsedIdx != 0 {
		t.Fatalf("retrieved addr index value didnt match expected")
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
		t.Fatalf("error getting address index: %v", err)
	}
	if idx != retrievedXPub.LastUsedIdx {
		t.Fatalf("retrieved addr index value didnt match expected")
	}
}
