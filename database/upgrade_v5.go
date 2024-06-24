// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"errors"
	"fmt"

	"github.com/decred/slog"
	bolt "go.etcd.io/bbolt"
)

func xPubBucketUpgrade(db *bolt.DB, log slog.Logger) error {
	log.Infof("Upgrading database to version %d", xPubBucketVersion)

	// feeXPub is the key which was used prior to this upgrade to store the xpub
	// in the root bucket.
	feeXPubK := []byte("feeXPub")

	// lastaddressindex is the key which was used prior to this upgrade to store
	// the index of the last used address in the root bucket.
	lastAddressIndexK := []byte("lastaddressindex")

	// Run the upgrade in a single database transaction so it can be safely
	// rolled back if an error is encountered.
	err := db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)
		ticketBkt := vspBkt.Bucket(ticketBktK)

		// Retrieve the current xpub.
		xpubBytes := vspBkt.Get(feeXPubK)
		if xpubBytes == nil {
			return errors.New("xpub not found")
		}
		feeXPub := string(xpubBytes)

		// Retrieve the current last addr index. Could be nil if this xpub was
		// never used.
		idxBytes := vspBkt.Get(lastAddressIndexK)
		var idx uint32
		if idxBytes != nil {
			idx = bytesToUint32(idxBytes)
		}

		// Delete the old values from the database.
		err := vspBkt.Delete(feeXPubK)
		if err != nil {
			return fmt.Errorf("could not delete xpub: %w", err)
		}
		err = vspBkt.Delete(lastAddressIndexK)
		if err != nil {
			return fmt.Errorf("could not delete last addr idx: %w", err)
		}

		// Insert the key into the bucket.
		newXpub := FeeXPub{
			ID:          0,
			Key:         feeXPub,
			LastUsedIdx: idx,
			Retired:     0,
		}
		err = insertFeeXPub(tx, newXpub)
		if err != nil {
			return fmt.Errorf("failed to store xpub in new bucket: %w", err)
		}

		// Update all existing tickets with xpub key ID 0.
		err = ticketBkt.ForEachBucket(func(k []byte) error {
			return ticketBkt.Bucket(k).Put(feeAddressXPubIDK, uint32ToBytes(0))
		})
		if err != nil {
			return fmt.Errorf("setting ticket xpub ID to 0 failed: %w", err)
		}

		// Update database version.
		err = vspBkt.Put(versionK, uint32ToBytes(xPubBucketVersion))
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
