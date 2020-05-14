package database

import (
	"encoding/binary"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// VspDatabase wraps an instance of bolt.DB and provides VSP specific
// convenience functions.
type VspDatabase struct {
	db *bolt.DB
}

// The keys used in the database.
var (
	// vspbkt is the main parent bucket of the VSP database. All values and
	// other buckets are nested within it.
	vspBktK = []byte("vspbkt")
	// ticketbkt stores all tickets known by this VSP.
	ticketBktK = []byte("ticketbkt")
	// version is the current database version.
	versionK = []byte("version")
)

// New initialises and returns a database. If no database file is found at the
// provided path, a new one will be created. Returns an open database which
// should always be closed after use.
func New(dbFile string) (*VspDatabase, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("unable to open db file: %v", err)
	}

	// Create all storage buckets of the VSP if they don't already exist.
	var newDB bool
	err = db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(vspBktK) == nil {
			newDB = true
			// Create parent bucket.
			vspBkt, err := tx.CreateBucket(vspBktK)
			if err != nil {
				return fmt.Errorf("failed to create %s bucket: %v", string(vspBktK), err)
			}

			// Initialise with database version 1.
			vbytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(vbytes, uint32(1))
			err = vspBkt.Put(versionK, vbytes)
			if err != nil {
				return err
			}

			// Create ticket bucket.
			_, err = vspBkt.CreateBucket(ticketBktK)
			if err != nil {
				return fmt.Errorf("failed to create %s bucket: %v", string(ticketBktK), err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if newDB {
		log.Debugf("Created new database %s", dbFile)
	} else {
		log.Debugf("Using existing database %s", dbFile)
	}

	return &VspDatabase{db: db}, nil
}

// Close releases all database resources. It will block waiting for any open
// transactions to finish before closing the database and returning.
func (vdb *VspDatabase) Close() error {
	log.Debug("Closing database")
	return vdb.db.Close()
}
