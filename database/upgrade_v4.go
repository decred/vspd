// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func altSignAddrUpgrade(db *bolt.DB) error {
	log.Infof("Upgrading database to version %d", altSignAddrVersion)

	// Run the upgrade in a single database transaction so it can be safely
	// rolled back if an error is encountered.
	err := db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		// Create alt sign addr bucket.
		_, err := vspBkt.CreateBucket(altSignAddrBktK)
		if err != nil {
			return fmt.Errorf("failed to create %s bucket: %w", altSignAddrBktK, err)
		}

		// Update database version.
		err = vspBkt.Put(versionK, uint32ToBytes(altSignAddrVersion))
		if err != nil {
			return fmt.Errorf("failed to update db version: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	log.Info("Upgrade completed")
	return nil
}
