package database

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
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
	db *bolt.DB

	ticketsMtx sync.RWMutex
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
	// feeXPub is the extended public key used for collecting VSP fees.
	feeXPubK = []byte("feeXPub")
	// cookieSecret is the secret key for initializing the cookie store.
	cookieSecretK = []byte("cookieSecret")
	// privatekey is the private key.
	privateKeyK = []byte("privatekey")
	// lastaddressindex is the index of the last address used for fees.
	lastAddressIndexK = []byte("lastaddressindex")
)

// backupMtx protects writeBackup, to ensure only one backup file is written at
// a time.
var backupMtx sync.Mutex

func writeBackup(db *bolt.DB) error {
	backupMtx.Lock()
	defer backupMtx.Unlock()

	backupPath := db.Path() + "-backup"
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

	log.Tracef("Database backup written to %s", backupPath)
	return err
}

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

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Database initialized")

	return nil
}

// Open initializes and returns an open database. If no database file is found
// at the provided path, a new one will be created.
func Open(ctx context.Context, shutdownWg *sync.WaitGroup, dbFile string, backupInterval time.Duration) (*VspDatabase, error) {

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
				err := writeBackup(db)
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

	return &VspDatabase{db: db}, nil
}

func (vdb *VspDatabase) Close() {
	err := writeBackup(vdb.db)
	if err != nil {
		log.Errorf("Failed to write database backup: %v", err)
	}

	err = vdb.db.Close()
	if err != nil {
		log.Errorf("Error closing database: %v", err)
	} else {
		log.Debug("Database closed")
	}
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
