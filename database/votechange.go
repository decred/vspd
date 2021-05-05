// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"encoding/json"
	"fmt"
	"math"

	bolt "go.etcd.io/bbolt"
)

// VoteChangeRecord is serialized to json and stored in bbolt db. The json keys
// are deliberately kept short because they are duplicated many times in the db.
type VoteChangeRecord struct {
	Request           string `json:"req"`
	RequestSignature  string `json:"reqs"`
	Response          string `json:"rsp"`
	ResponseSignature string `json:"rsps"`
}

// SaveVoteChange will insert the provided vote change record into the database,
// and if this breaches the maximum amount of allowed records, delete the oldest
// one which is currently stored. Records are stored using a serially increasing
// integer as the key.
func (vdb *VspDatabase) SaveVoteChange(ticketHash string, record VoteChangeRecord) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {

		// Create or get a bucket for this ticket.
		bkt, err := tx.Bucket(vspBktK).Bucket(voteChangeBktK).
			CreateBucketIfNotExists([]byte(ticketHash))
		if err != nil {
			return fmt.Errorf("failed to create vote change bucket (ticketHash=%s): %w",
				ticketHash, err)
		}

		// Loop through the bucket to count the records, as well as finding the
		// most recent and the oldest record.
		var count int
		highest := uint32(0)
		lowest := uint32(math.MaxUint32)
		err = bkt.ForEach(func(k, v []byte) error {
			count++
			key := bytesToUint32(k)
			if key > highest {
				highest = key
			}
			if key < lowest {
				lowest = key
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error iterating over vote change bucket: %w", err)
		}

		// If bucket is at (or over) the limit of max allowed records, remove
		// the oldest one.
		if count >= vdb.maxVoteChangeRecords {
			err = bkt.Delete(uint32ToBytes(lowest))
			if err != nil {
				return fmt.Errorf("failed to delete old vote change record: %w", err)
			}
		}

		// Insert record with index 0 if the bucket is currently empty,
		// otherwise use most recent + 1.
		var newKey uint32
		if count > 0 {
			newKey = highest + 1
		}

		// Insert record.
		recordBytes, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("could not marshal vote change record: %w", err)
		}
		err = bkt.Put(uint32ToBytes(newKey), recordBytes)
		if err != nil {
			return fmt.Errorf("could not store vote change record: %w", err)
		}

		return nil
	})
}

// GetVoteChanges retrieves all of the stored vote change records for the
// provided ticket hash.
func (vdb *VspDatabase) GetVoteChanges(ticketHash string) (map[uint32]VoteChangeRecord, error) {

	records := make(map[uint32]VoteChangeRecord)

	err := vdb.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(vspBktK).Bucket(voteChangeBktK).
			Bucket([]byte(ticketHash))

		if bkt == nil {
			return nil
		}

		err := bkt.ForEach(func(k, v []byte) error {
			var record VoteChangeRecord
			err := json.Unmarshal(v, &record)
			if err != nil {
				return fmt.Errorf("could not unmarshal vote change record: %w", err)
			}

			records[bytesToUint32(k)] = record

			return nil
		})
		if err != nil {
			return fmt.Errorf("error iterating over vote change bucket: %w", err)
		}

		return nil
	})

	return records, err
}
