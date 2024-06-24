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
