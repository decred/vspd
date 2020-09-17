// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// VspDatabase wraps an instance of bolt.DB and provides VSP specific
// convenience functions.
type VspDatabase struct {
	db                   *bolt.DB
	maxVoteChangeRecords int

	ticketsMtx sync.RWMutex
}

// The keys used in the database.
var (
	// vspbkt is the main parent bucket of the VSP database. All values and
	// other buckets are nested within it.
	vspBktK = []byte("vspbkt")
	// ticketbkt stores all tickets known by this VSP.
	ticketBktK = []byte("ticketbkt")
	// votechangebkt stores records of web requests which update vote choices.
	voteChangeBktK = []byte("votechangebkt")
	// version is the current database version.
	versionK = []byte("version")
	// feeXPub is the extended public key used for collecting VSP fees.
	feeXPubK = []byte("feeXPub")
	// cookieSecret is the secret key for initializing the cookie store.
	cookieSecretK = []byte("cookieSecret")
	// privatekey is the private key.
	privateKeyK = []byte("privatekey")
	// lastaddressindex is the index of the last address used for fees.
	lastAddressIndexK = []byte("lastaddressindex")
)

const (
	// backupFileMode is the file mode for database backup files written by vspd.
	backupFileMode = 0600
)

// backupMtx should be held when writing to the database backup file
var backupMtx sync.Mutex

// writeHotBackupFile writes a backup of the database file while the database
// is still open.
func writeHotBackupFile(db *bolt.DB) error {
	backupMtx.Lock()
	defer backupMtx.Unlock()

	backupPath := db.Path() + "-backup"
	tempPath := backupPath + "~"

	// Write backup to temporary file.
	err := db.View(func(tx *bolt.Tx) error {
		return tx.CopyFile(tempPath, backupFileMode)
	})
	if err != nil {
		return fmt.Errorf("tx.CopyFile: %v", err)
	}

	// Rename temporary file to actual backup file.
	err = os.Rename(tempPath, backupPath)
	if err != nil {
		return fmt.Errorf("os.Rename: %v", err)
	}

	log.Tracef("Database backup written to %s", backupPath)
	return err
}

// CreateNew intializes a new bbolt database with all of the necessary vspd
// buckets, and inserts:
// - the provided extended pubkey (to be used for deriving fee addresses).
// - an ed25519 keypair to sign API responses.
// - a secret key to use for initializing a HTTP cookie store.
func CreateNew(dbFile, feeXPub string) error {
	log.Infof("Initializing new database at %s", dbFile)

	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return fmt.Errorf("unable to open db file: %v", err)
	}

	defer db.Close()

	// Create all storage buckets of the VSP if they don't already exist.
	err = db.Update(func(tx *bolt.Tx) error {
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

		log.Info("Generating ed25519 signing key")

		// Generate ed25519 key
		_, signKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return fmt.Errorf("failed to generate signing key: %v", err)
		}
		err = vspBkt.Put(privateKeyK, signKey.Seed())
		if err != nil {
			return err
		}

		// Generate a secret key for initializing the cookie store.
		log.Info("Generating cookie secret")
		secret := make([]byte, 32)
		_, err = rand.Read(secret)
		if err != nil {
			return err
		}
		err = vspBkt.Put(cookieSecretK, secret)
		if err != nil {
			return err
		}

		log.Info("Storing extended public key")
		// Store fee xpub
		err = vspBkt.Put(feeXPubK, []byte(feeXPub))
		if err != nil {
			return err
		}

		// Create ticket bucket.
		_, err = vspBkt.CreateBucket(ticketBktK)
		if err != nil {
			return fmt.Errorf("failed to create %s bucket: %v", string(ticketBktK), err)
		}

		// Create vote change bucket.
		_, err = vspBkt.CreateBucket(voteChangeBktK)
		if err != nil {
			return fmt.Errorf("failed to create %s bucket: %v", string(voteChangeBktK), err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Database initialized")

	return nil
}

// Open initializes and returns an open database. An error is returned if no
// database file is found at the provided path.
func Open(ctx context.Context, shutdownWg *sync.WaitGroup, dbFile string, backupInterval time.Duration, maxVoteChangeRecords int) (*VspDatabase, error) {

	// Error if db file does not exist. This is needed because bolt.Open will
	// silently create a new empty database if the file does not exist. A new
	// vspd database should be created with the CreateNew() function.
	_, err := os.Stat(dbFile)
	if os.IsNotExist(err) {
		return nil, err
	}

	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("unable to open db file: %v", err)
	}

	log.Debugf("Opened database file %s", dbFile)

	// Start a ticker to update the backup file at the specified interval.
	shutdownWg.Add(1)
	go func() {
		ticker := time.NewTicker(backupInterval)
		for {
			select {
			case <-ticker.C:
				err := writeHotBackupFile(db)
				if err != nil {
					log.Errorf("Failed to write database backup: %v", err)
				}
			case <-ctx.Done():
				ticker.Stop()
				shutdownWg.Done()
				return
			}
		}
	}()

	return &VspDatabase{db: db, maxVoteChangeRecords: maxVoteChangeRecords}, nil
}

// Close will close the database and then make a copy of the database to the
// backup location.
func (vdb *VspDatabase) Close() {

	// Make a copy of the db path here because once the db is closed, db.Path
	// returns empty string.
	dbPath := vdb.db.Path()

	// Close will wait until all on-going transactions are completed before
	// closing the db and writing the file to disk.
	err := vdb.db.Close()
	if err != nil {
		log.Errorf("Error closing database: %v", err)
		// Return here because if there is an issue with the database, we
		// probably don't want to overwrite the backup file and potentially
		// break that too.
		return
	}

	log.Debug("Database closed")

	// Ensure the database backup file is up-to-date.
	backupPath := dbPath + "-backup"
	tempPath := backupPath + "~"

	backupMtx.Lock()
	defer backupMtx.Unlock()

	from, err := os.Open(dbPath)
	if err != nil {
		log.Errorf("Failed to write a database backup (os.Open): %v", err)
		return
	}
	defer from.Close()

	to, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE, backupFileMode)
	if err != nil {
		log.Errorf("Failed to write a database backup (os.OpenFile): %v", err)
		return
	}
	defer to.Close()

	_, err = io.Copy(to, from)
	if err != nil {
		log.Errorf("Failed to write a database backup (io.Copy): %v", err)
		return
	}

	// Rename temporary file to actual backup file.
	err = os.Rename(tempPath, backupPath)
	if err != nil {
		log.Errorf("Failed to write a database backup (os.Rename): %v", err)
		return
	}

	log.Tracef("Database backup written to %s", backupPath)
}

// KeyPair retrieves the keypair used to sign API responses from the database.
func (vdb *VspDatabase) KeyPair() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	var seed []byte
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		s := vspBkt.Get(privateKeyK)

		// Byte slices returned from Bolt are only valid during a transaction.
		// Need to make a copy.
		seed = make([]byte, len(s))
		copy(seed, s)

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

// GetFeeXPub retrieves the extended pubkey used for generating fee addresses
// from the database.
func (vdb *VspDatabase) GetFeeXPub() (string, error) {
	var feeXPub string
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		xpubBytes := vspBkt.Get(feeXPubK)
		if xpubBytes == nil {
			return nil
		}

		feeXPub = string(xpubBytes)

		return nil
	})

	return feeXPub, err
}

// GetCookieSecret retrieves the generated cookie store secret key from the
// database.
func (vdb *VspDatabase) GetCookieSecret() ([]byte, error) {
	var cookieSecret []byte
	err := vdb.db.View(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)

		cs := vspBkt.Get(cookieSecretK)

		// Byte slices returned from Bolt are only valid during a transaction.
		// Need to make a copy.
		cookieSecret = make([]byte, len(cs))
		copy(cookieSecret, cs)

		return nil
	})

	return cookieSecret, err
}

// BackupDB streams a backup of the database over an http response writer.
func (vdb *VspDatabase) BackupDB(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="vspd.db"`)

	err := vdb.db.View(func(tx *bolt.Tx) error {
		w.Header().Set("Content-Length", strconv.Itoa(int(tx.Size())))
		_, err := tx.WriteTo(w)
		return err
	})

	return err
}
