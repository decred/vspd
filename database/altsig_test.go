// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
)

func exampleAltSigData() *AltSigData {
	return &AltSigData{
		AltSigAddr: randString(35, addrCharset),
		Req:        randBytes(1000),
		ReqSig:     randString(96, sigCharset),
		Res:        randBytes(1000),
		ResSig:     randString(96, sigCharset),
	}
}

func ensureData(t *testing.T, ticketHash string, wantData *AltSigData) {
	t.Helper()

	data, err := db.AltSigData(ticketHash)
	if err != nil {
		t.Fatalf("unexpected error fetching alt signature data: %v", err)
	}
	if !reflect.DeepEqual(wantData, data) {
		t.Fatal("want data different than actual")
	}
}

func testAltSigData(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Not added yet so no values should exist in the db.
	h, err := db.AltSigData(ticketHash)
	if err != nil {
		t.Fatalf("unexpected error fetching alt signature data: %v", err)
	}
	if h != nil {
		t.Fatal("expected no data")
	}

	// Insert an altsig.
	data := exampleAltSigData()
	if err := db.InsertAltSig(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing altsig in database: %v", err)
	}

	ensureData(t, ticketHash, data)
}

func testInsertAltSig(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Not added yet so no values should exist in the db.
	ensureData(t, ticketHash, nil)

	data := exampleAltSigData()
	// Clear alt sig addr for test.
	data.AltSigAddr = ""

	if err := db.InsertAltSig(ticketHash, data); err == nil {
		t.Fatalf("expected error for insert blank address")
	}

	if err := db.InsertAltSig(ticketHash, nil); err == nil {
		t.Fatalf("expected error for nil data")
	}

	// Still no change on errors.
	ensureData(t, ticketHash, nil)

	// Re-add alt sig addr.
	data.AltSigAddr = randString(35, addrCharset)

	// Insert an altsig.
	if err := db.InsertAltSig(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing altsig in database: %v", err)
	}

	ensureData(t, ticketHash, data)

	// Further additions should error and not change the data.
	secondData := exampleAltSigData()
	secondData.AltSigAddr = data.AltSigAddr
	if err := db.InsertAltSig(ticketHash, secondData); err == nil {
		t.Fatalf("expected error for second altsig addition")
	}

	ensureData(t, ticketHash, data)
}

func testDeleteAltSig(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Nothing to delete.
	if err := db.DeleteAltSig(ticketHash); err != nil {
		t.Fatalf("unexpected error deleting nonexistant altsig")
	}

	// Insert an altsig.
	data := exampleAltSigData()
	if err := db.InsertAltSig(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing altsig in database: %v", err)
	}

	ensureData(t, ticketHash, data)

	if err := db.DeleteAltSig(ticketHash); err != nil {
		t.Fatalf("unexpected error deleting altsig: %v", err)
	}

	ensureData(t, ticketHash, nil)
}
