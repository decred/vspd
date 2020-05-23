package database

import (
	"encoding/binary"

	bolt "go.etcd.io/bbolt"
)

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

func (vdb *VspDatabase) SetLastAddressIndex(idx uint32) error {
	err := vdb.db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		idxBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(idxBytes, idx)

		return vspBkt.Put(lastAddressIndexK, idxBytes)

	})

	return err
}
