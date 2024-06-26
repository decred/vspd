// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// FeeXPub is serialized to json and stored in bbolt db.
type FeeXPub struct {
	ID          uint32 `json:"id"`
	Key         string `json:"key"`
	LastUsedIdx uint32 `json:"lastusedidx"`
	// Retired is a unix timestamp representing the moment the key was retired,
	// or zero for the currently active key.
	Retired int64 `json:"retired"`
}

// insertFeeXPub stores the provided pubkey in the database, regardless of
// whether a value pre-exists.
func insertFeeXPub(tx *bolt.Tx, xpub FeeXPub) error {
	vspBkt := tx.Bucket(vspBktK)

	keyBkt, err := vspBkt.CreateBucketIfNotExists(xPubBktK)
	if err != nil {
		return fmt.Errorf("failed to get %s bucket: %w", string(xPubBktK), err)
	}

	keyBytes, err := json.Marshal(xpub)
	if err != nil {
		return fmt.Errorf("could not marshal xpub: %w", err)
	}

	err = keyBkt.Put(uint32ToBytes(xpub.ID), keyBytes)
	if err != nil {
		return fmt.Errorf("could not store xpub: %w", err)
	}

	return nil
}

// FeeXPub retrieves the currently active extended pubkey used for generating
// fee addresses from the database.
func (vdb *VspDatabase) FeeXPub() (FeeXPub, error) {
	xpubs, err := vdb.AllXPubs()
	if err != nil {
		return FeeXPub{}, err
	}

	// Find the active xpub - the one with the highest ID.
	var highest uint32
	for id := range xpubs {
		if id > highest {
			highest = id
		}
	}

	return xpubs[highest], nil
}

// RetireXPub will mark the currently active xpub key as retired and insert the
// provided pubkey as the currently active one.
func (vdb *VspDatabase) RetireXPub(xpub string) error {
	// Ensure the new xpub has never been used before.
	xpubs, err := vdb.AllXPubs()
	if err != nil {
		return err
	}
	for _, x := range xpubs {
		if x.Key == xpub {
			return errors.New("provided xpub has already been used")
		}
	}

	current, err := vdb.FeeXPub()
	if err != nil {
		return err
	}
	current.Retired = time.Now().Unix()

	return vdb.db.Update(func(tx *bolt.Tx) error {
		// Store the retired xpub.
		err := insertFeeXPub(tx, current)
		if err != nil {
			return err
		}

		// Insert new xpub.
		newKey := FeeXPub{
			ID:          current.ID + 1,
			Key:         xpub,
			LastUsedIdx: 0,
			Retired:     0,
		}
		err = insertFeeXPub(tx, newKey)
		if err != nil {
			return err
		}

		return nil
	})
}

// AllXPubs retrieves the current and any retired extended pubkeys from the
// database.
func (vdb *VspDatabase) AllXPubs() (map[uint32]FeeXPub, error) {
	xpubs := make(map[uint32]FeeXPub)

	err := vdb.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(vspBktK).Bucket(xPubBktK)

		if bkt == nil {
			return fmt.Errorf("%s bucket doesn't exist", string(xPubBktK))
		}

		err := bkt.ForEach(func(k, v []byte) error {
			var xpub FeeXPub
			err := json.Unmarshal(v, &xpub)
			if err != nil {
				return fmt.Errorf("could not unmarshal xpub key: %w", err)
			}

			xpubs[bytesToUint32(k)] = xpub

			return nil
		})
		if err != nil {
			return fmt.Errorf("error iterating over %s bucket: %w", string(xPubBktK), err)
		}

		return nil
	})

	return xpubs, err
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
