// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// The keys used to store the altsig in the database.
var (
	altSigAddrK = []byte("altsig")
	reqK        = []byte("req")
	reqSigK     = []byte("reqsig")
	resK        = []byte("res")
	resSigK     = []byte("ressig")
)

// AltSigData holds the information needed to prove that a client added an
// alternate signature address.
type AltSigData struct {
	// AltSigAddr is the new alternate signature address. It is base 58
	// encoded.
	AltSigAddr string
	// Req is the original request to set an alternate signature.
	Req []byte
	// ReqSig is the request's signature signed by the private key that
	// corresponds to the address. It is base 64 encoded.
	ReqSig string
	// Res is the original response from the server to the alternate
	// signature address.
	Res []byte
	// ResSig is the response's signature signed by the server. It is base
	// 64 encoded.
	ResSig string
}

// InsertAltSig will insert the provided ticket into the database. Returns an
// error if data for the ticket hash already exist.
//
// Passed data must have no empty fields.
func (vdb *VspDatabase) InsertAltSig(ticketHash string, data *AltSigData) error {
	if data == nil {
		return errors.New("alt sig data must not be nil for inserts")
	}

	if data.AltSigAddr == "" || len(data.Req) == 0 || data.ReqSig == "" ||
		len(data.Res) == 0 || data.ResSig == "" {
		return errors.New("alt sig data has empty parameters")
	}

	return vdb.db.Update(func(tx *bolt.Tx) error {
		altSigBkt := tx.Bucket(vspBktK).Bucket(altSigBktK)

		// Create a bucket for the new altsig. Returns an error if bucket
		// already exists.
		bkt, err := altSigBkt.CreateBucket([]byte(ticketHash))
		if err != nil {
			return fmt.Errorf("could not create bucket for altsig: %w", err)
		}

		if err := bkt.Put(altSigAddrK, []byte(data.AltSigAddr)); err != nil {
			return err
		}

		if err := bkt.Put(reqK, data.Req); err != nil {
			return err
		}

		if err := bkt.Put(reqSigK, []byte(data.ReqSig)); err != nil {
			return err
		}

		if err := bkt.Put(resK, data.Res); err != nil {
			return err
		}
		return bkt.Put(resSigK, []byte(data.ResSig))
	})
}

// DeleteAltSig deletes an altsig from the database. Does not error if there is
// no altsig in the database.
func (vdb *VspDatabase) DeleteAltSig(ticketHash string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		altSigBkt := tx.Bucket(vspBktK).Bucket(altSigBktK)

		// Don't attempt delete if doesn't exist.
		bkt := altSigBkt.Bucket([]byte(ticketHash))
		if bkt == nil {
			return nil
		}

		err := altSigBkt.DeleteBucket([]byte(ticketHash))
		if err != nil {
			return fmt.Errorf("could not delete altsig: %w", err)
		}

		return nil
	})
}

// AltSigData retrieves a ticket's alternate signature data. Existence of an
// alternate signature can be inferred by no error and nil data return.
func (vdb *VspDatabase) AltSigData(ticketHash string) (*AltSigData, error) {
	var h *AltSigData
	return h, vdb.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(vspBktK).Bucket(altSigBktK).Bucket([]byte(ticketHash))
		if bkt == nil {
			return nil
		}
		h = &AltSigData{
			AltSigAddr: string(bkt.Get(altSigAddrK)),
			Req:        bkt.Get(reqK),
			ReqSig:     string(bkt.Get(reqSigK)),
			Res:        bkt.Get(resK),
			ResSig:     string(bkt.Get(resSigK)),
		}
		return nil
	})
}
