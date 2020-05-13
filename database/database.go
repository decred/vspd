package database

import (
	"encoding/binary"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// VspDatabase wraps an instance of bbolt DB and provides VSP specific
// convenience functions.
type VspDatabase struct {
	db *bolt.DB
}

var (
	// vspBkt is the main parent bucket of the VSP. All values and other buckets
	// are nested within it.
	vspBkt     = []byte("vspbkt")
	feesBkt    = []byte("feesbkt")
	versionK   = []byte("version")
	backupFile = "backup.kv"
	version    = 1
)

// New initialises and returns a database connection. If no database file is
// found at the provided path, a new one will be created. Returns an open
// database connection which should be closed after use.
func New(dbFile string) (*VspDatabase, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("unable to open db file: %v", err)
	}

	err = createBuckets(db)
	if err != nil {
		return nil, err
	}

	return &VspDatabase{db: db}, nil
}

// createBuckets creates all storage buckets of the VSP.
func createBuckets(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		pbkt := tx.Bucket(feesBkt)
		if pbkt == nil {
			pbkt, err := tx.CreateBucket(feesBkt)
			if err != nil {
				return fmt.Errorf("failed to create %s bucket: %v", string(feesBkt), err)
			}

			vbytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(vbytes, uint32(version))
			err = pbkt.Put(versionK, vbytes)
			if err != nil {
				return err
			}
		}

		return nil
	})
}
