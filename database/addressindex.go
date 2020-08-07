package database

import (
	"encoding/binary"

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

		idx = binary.LittleEndian.Uint32(idxBytes)

		return nil
	})

	return idx, err
}

// SetLastAddressIndex updates the last index used to derive a new fee address
// from the fee xpub key.
func (vdb *VspDatabase) SetLastAddressIndex(idx uint32) error {
	err := vdb.db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		idxBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(idxBytes, idx)

		return vspBkt.Put(lastAddressIndexK, idxBytes)

	})

	return err
}
