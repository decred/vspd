package database

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
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
	// privatekey is the private key.
	privateKeyK = []byte("privatekey")
	// lastaddressindex is the index of the last address used for fees.
	lastAddressIndexK = []byte("lastaddressindex")
)

// backupMtx protects writeBackup, to ensure only one backup file is written at
// a time.
var backupMtx sync.Mutex

func writeBackup(db *bolt.DB, dbFile string) error {
	backupMtx.Lock()
	defer backupMtx.Unlock()

	backupPath := dbFile + "-backup"
	tempPath := backupPath + "~"

	// Write backup to temporary file.
	err := db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(tempPath, 0600)
	})
	if err != nil {
		return fmt.Errorf("tx.CopyFile: %v", err)
	}

	// Rename temporary file to actual backup file.
	err = os.Rename(tempPath, backupPath)
	if err != nil {
		return fmt.Errorf("os.Rename: %v", err)
	}

	log.Debugf("Database backup written to %s", backupPath)
	return err
}

// Open initializes and returns an open database. If no database file is found
// at the provided path, a new one will be created.
func Open(ctx context.Context, shutdownWg *sync.WaitGroup, dbFile string, backupInterval time.Duration) (*VspDatabase, error) {

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

		err := writeBackup(db, dbFile)
		if err != nil {
			log.Errorf("Failed to write database backup: %v", err)
		}

		err = db.Close()
		if err != nil {
			log.Errorf("Error closing database: %v", err)
		} else {
			log.Debug("Database closed")
		}
		shutdownWg.Done()
	}()

	// Start a ticker to update the backup file at the specified interval.
	shutdownWg.Add(1)
	backupTicker := time.NewTicker(backupInterval)
	go func() {
		for {
			select {
			case <-backupTicker.C:
				err := writeBackup(db, dbFile)
				if err != nil {
					log.Errorf("Failed to write database backup: %v", err)
				}
			case <-ctx.Done():
				backupTicker.Stop()
				shutdownWg.Done()
				return
			}
		}
	}()

	// Create all storage buckets of the VSP if they don't already exist.
	err = db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket(vspBktK) == nil {
			log.Debug("Initializing new database")
			// Create parent bucket.
			vspBkt, err := tx.CreateBucket(vspBktK)
			if err != nil {
				return fmt.Errorf("failed to create %s bucket: %v", string(vspBktK), err)
			}

			// Initialize with database version 1.
			vbytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(vbytes, uint32(1))
			err = vspBkt.Put(versionK, vbytes)
			if err != nil {
				return err
			}

			// Generate ed25519 key
			_, signKey, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				return fmt.Errorf("failed to generate signing key: %v", err)
			}
			err = vspBkt.Put(privateKeyK, signKey.Seed())
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

func (vdb *VspDatabase) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	var seed []byte
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		seed = vspBkt.Get(privateKeyK)
		if seed == nil {
			// should not happen
			return fmt.Errorf("no private key found")
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	signKey := ed25519.NewKeyFromSeed(seed)

	// Derive pubKey from signKey
	pubKey, ok := signKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("failed to cast signing key: %T", pubKey)
	}

	return signKey, pubKey, err
}
