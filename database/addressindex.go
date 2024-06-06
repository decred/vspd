// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	bolt "go.etcd.io/bbolt"
)

// insertFeeXPub stores the provided pubkey in the database, regardless of
// whether a value pre-exists.
func insertFeeXPub(tx *bolt.Tx, feeXPub string) error {
	return tx.Bucket(vspBktK).Put(feeXPubK, []byte(feeXPub))
}

// FeeXPub retrieves the extended pubkey used for generating fee addresses
// from the database.
func (vdb *VspDatabase) FeeXPub() (string, error) {
	var feeXPub string
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		xpubBytes := vspBkt.Get(feeXPubK)
		if xpubBytes == nil {
			return nil
		}

		feeXPub = string(xpubBytes)

		return nil
	})

	return feeXPub, err
}

// GetLastAddressIndex retrieves the last index used to derive a new fee
// address from the fee xpub key.
func (vdb *VspDatabase) GetLastAddressIndex() (uint32, error) {
	var idx uint32
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		idxBytes := vspBkt.Get(lastAddressIndexK)
		if idxBytes == nil {
			return nil
		}

		idx = bytesToUint32(idxBytes)

		return nil
	})

	return idx, err
}

// SetLastAddressIndex updates the last index used to derive a new fee address
// from the fee xpub key.
func (vdb *VspDatabase) SetLastAddressIndex(idx uint32) error {
	err := vdb.db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)
		return vspBkt.Put(lastAddressIndexK, uint32ToBytes(idx))
	})

	return err
}
