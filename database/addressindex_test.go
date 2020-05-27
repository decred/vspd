package database

import (
	"testing"
)

func testAddressIndex(t *testing.T) {

	// Getting index before it has been set should return 0.
	idx, err := db.GetLastAddressIndex()
	if err != nil {
		t.Fatalf("error getting address index: %v", err)
	}
	if idx != 0 {
		t.Fatalf("retrieved addr index value didnt match expected")
	}

	// Update address index.
	idx = uint32(99)
	err = db.SetLastAddressIndex(idx)
	if err != nil {
		t.Fatalf("error setting address index: %v", err)
	}

	// Check for updated value.
	retrievedIdx, err := db.GetLastAddressIndex()
	if err != nil {
		t.Fatalf("error getting address index: %v", err)
	}
	if idx != retrievedIdx {
		t.Fatalf("retrieved addr index value didnt match expected")
	}
}
