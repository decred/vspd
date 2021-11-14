// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// The keys used to store alternate signing addresses in the database.
var (
	altSignAddrK = []byte("altsig")
	reqK         = []byte("req")
	reqSigK      = []byte("reqsig")
	respK        = []byte("res")
	respSigK     = []byte("ressig")
)

// AltSignAddrData holds the information needed to prove that a client added an
// alternate signing address.
type AltSignAddrData struct {
	// AltSignAddr is the new alternate signing address. It is base 58 encoded.
	AltSignAddr string
	// Req is the original request to set an alternate signing address.
	Req []byte
	// ReqSig is the request's signature signed by the commitment address of the
	// corresponding ticket. It is base 64 encoded.
	ReqSig string
	// Resp is the original response from the server to the alternate signing
	// address.
	Resp []byte
	// RespSig is the response's signature signed by the server. It is base 64
	// encoded.
	RespSig string
}

// InsertAltSignAddr will insert the provided alternate signing address into the
// database. Returns an error if data for the ticket hash already exist.
//
// Passed data must have no empty fields.
func (vdb *VspDatabase) InsertAltSignAddr(ticketHash string, data *AltSignAddrData) error {
	if data == nil {
		return errors.New("alt sign addr data must not be nil for inserts")
	}

	if data.AltSignAddr == "" || len(data.Req) == 0 || data.ReqSig == "" ||
		len(data.Resp) == 0 || data.RespSig == "" {
		return errors.New("alt sign addr data has empty parameters")
	}

	return vdb.db.Update(func(tx *bolt.Tx) error {
		altSignAddrBkt := tx.Bucket(vspBktK).Bucket(altSignAddrBktK)

		// Create a bucket for the new alt sign addr. Returns an error if bucket
		// already exists.
		bkt, err := altSignAddrBkt.CreateBucket([]byte(ticketHash))
		if err != nil {
			return fmt.Errorf("could not create bucket for alt sign addr: %w", err)
		}

		if err := bkt.Put(altSignAddrK, []byte(data.AltSignAddr)); err != nil {
			return err
		}

		if err := bkt.Put(reqK, data.Req); err != nil {
			return err
		}

		if err := bkt.Put(reqSigK, []byte(data.ReqSig)); err != nil {
			return err
		}

		if err := bkt.Put(respK, data.Resp); err != nil {
			return err
		}
		return bkt.Put(respSigK, []byte(data.RespSig))
	})
}

// DeleteAltSignAddr deletes an alternate signing address from the database.
// Does not error if there is no record in the database to delete.
func (vdb *VspDatabase) DeleteAltSignAddr(ticketHash string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		altSignAddrBkt := tx.Bucket(vspBktK).Bucket(altSignAddrBktK)

		// Don't attempt delete if doesn't exist.
		bkt := altSignAddrBkt.Bucket([]byte(ticketHash))
		if bkt == nil {
			return nil
		}

		err := altSignAddrBkt.DeleteBucket([]byte(ticketHash))
		if err != nil {
			return fmt.Errorf("could not delete altsignaddr: %w", err)
		}

		return nil
	})
}

// AltSignAddrData retrieves a ticket's alternate signing data. Existence of an
// alternate signing address can be inferred by no error and nil data return.
func (vdb *VspDatabase) AltSignAddrData(ticketHash string) (*AltSignAddrData, error) {
	var h *AltSignAddrData
	return h, vdb.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(vspBktK).Bucket(altSignAddrBktK).Bucket([]byte(ticketHash))
		if bkt == nil {
			return nil
		}
		h = &AltSignAddrData{
			AltSignAddr: string(bkt.Get(altSignAddrK)),
			Req:         bkt.Get(reqK),
			ReqSig:      string(bkt.Get(reqSigK)),
			Resp:        bkt.Get(respK),
			RespSig:     string(bkt.Get(respSigK)),
		}
		return nil
	})
}
