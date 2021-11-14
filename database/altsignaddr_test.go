// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
)

func exampleAltSignAddrData() *AltSignAddrData {
	return &AltSignAddrData{
		AltSignAddr: randString(35, addrCharset),
		Req:         randBytes(1000),
		ReqSig:      randString(96, sigCharset),
		Resp:        randBytes(1000),
		RespSig:     randString(96, sigCharset),
	}
}

// ensureData will confirm that the provided data exists in the database.
func ensureData(t *testing.T, ticketHash string, wantData *AltSignAddrData) {
	t.Helper()

	data, err := db.AltSignAddrData(ticketHash)
	if err != nil {
		t.Fatalf("unexpected error fetching alt sign address data: %v", err)
	}
	if !reflect.DeepEqual(wantData, data) {
		t.Fatal("want data different than actual")
	}
}

func testAltSignAddrData(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Not added yet so no values should exist in the db.
	h, err := db.AltSignAddrData(ticketHash)
	if err != nil {
		t.Fatalf("unexpected error fetching alt sign address data: %v", err)
	}
	if h != nil {
		t.Fatal("expected no data")
	}

	// Insert an alt sign address.
	data := exampleAltSignAddrData()
	if err := db.InsertAltSignAddr(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing alt sign addr in database: %v", err)
	}

	ensureData(t, ticketHash, data)
}

func testInsertAltSignAddr(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Not added yet so no values should exist in the db.
	ensureData(t, ticketHash, nil)

	data := exampleAltSignAddrData()
	// Clear alt sign addr for test.
	data.AltSignAddr = ""

	if err := db.InsertAltSignAddr(ticketHash, data); err == nil {
		t.Fatalf("expected error for insert blank address")
	}

	if err := db.InsertAltSignAddr(ticketHash, nil); err == nil {
		t.Fatalf("expected error for nil data")
	}

	// Still no change on errors.
	ensureData(t, ticketHash, nil)

	// Re-add alt sig addr.
	data.AltSignAddr = randString(35, addrCharset)

	// Insert an alt sign addr.
	if err := db.InsertAltSignAddr(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing alt sig addr in database: %v", err)
	}

	ensureData(t, ticketHash, data)

	// Further additions should error and not change the data.
	secondData := exampleAltSignAddrData()
	secondData.AltSignAddr = data.AltSignAddr
	if err := db.InsertAltSignAddr(ticketHash, secondData); err == nil {
		t.Fatalf("expected error for second alt sig addr addition")
	}

	ensureData(t, ticketHash, data)
}

func testDeleteAltSignAddr(t *testing.T) {
	ticketHash := randString(64, hexCharset)

	// Nothing to delete.
	if err := db.DeleteAltSignAddr(ticketHash); err != nil {
		t.Fatalf("unexpected error deleting nonexistant alt sign addr")
	}

	// Insert an alt sign addr.
	data := exampleAltSignAddrData()
	if err := db.InsertAltSignAddr(ticketHash, data); err != nil {
		t.Fatalf("unexpected error storing alt sign addr in database: %v", err)
	}

	ensureData(t, ticketHash, data)

	if err := db.DeleteAltSignAddr(ticketHash); err != nil {
		t.Fatalf("unexpected error deleting alt sign addr: %v", err)
	}

	ensureData(t, ticketHash, nil)
}
