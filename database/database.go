package database

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
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

// Open initialises and returns an open database. If no database file is found
// at the provided path, a new one will be created.
func Open(ctx context.Context, shutdownWg *sync.WaitGroup, dbFile string) (*VspDatabase, error) {

	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("unable to open db file: %v", err)
	}

	log.Debugf("Opened database file %s", dbFile)

	// Add the graceful shutdown to the waitgroup.
	shutdownWg.Add(1)
	go func() {
		// Wait until shutdown is signaled before shutting down.
		<-ctx.Done()

		log.Debug("Closing database...")
		err := db.Close()
		if err != nil {
			log.Errorf("Error closing database: %v", err)
		} else {
			log.Debug("Database closed")
		}
		shutdownWg.Done()
	}()

	// Create all storage buckets of the VSP if they don't already exist.
	err = db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(vspBktK) == nil {
			log.Debug("Initialising new database")
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

	return &VspDatabase{db: db}, nil
}
