// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	bolt "go.etcd.io/bbolt"
)

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
