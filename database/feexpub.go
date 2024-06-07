// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

type FeeXPub struct {
	Key         string
	LastUsedIdx uint32
}

// insertFeeXPub stores the provided pubkey in the database, regardless of
// whether a value pre-exists.
func insertFeeXPub(tx *bolt.Tx, xpub FeeXPub) error {
	vspBkt := tx.Bucket(vspBktK)

	err := vspBkt.Put(feeXPubK, []byte(xpub.Key))
	if err != nil {
		return err
	}

	return vspBkt.Put(lastAddressIndexK, uint32ToBytes(xpub.LastUsedIdx))
}

// FeeXPub retrieves the extended pubkey used for generating fee addresses
// from the database.
func (vdb *VspDatabase) FeeXPub() (FeeXPub, error) {
	var feeXPub string
	var idx uint32
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		// Get the key.
		xpubBytes := vspBkt.Get(feeXPubK)
		if xpubBytes == nil {
			return nil
		}
		feeXPub = string(xpubBytes)

		// Get the last used address index.
		idxBytes := vspBkt.Get(lastAddressIndexK)
		if idxBytes == nil {
			return nil
		}
		idx = bytesToUint32(idxBytes)

		return nil
	})
	if err != nil {
		return FeeXPub{}, fmt.Errorf("could not retrieve fee xpub: %w", err)
	}

	return FeeXPub{Key: feeXPub, LastUsedIdx: idx}, nil
}

// SetLastAddressIndex updates the last index used to derive a new fee address
// from the fee xpub key.
func (vdb *VspDatabase) SetLastAddressIndex(idx uint32) error {
	current, err := vdb.FeeXPub()
	if err != nil {
		return err
	}

	current.LastUsedIdx = idx

	return vdb.db.Update(func(tx *bolt.Tx) error {
		return insertFeeXPub(tx, current)
	})
}
