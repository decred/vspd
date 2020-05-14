package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jrick/wsrpc/v2"
)

const listen = ":3000"

type Config struct {
	signKey   ed25519.PrivateKey
	pubKey    ed25519.PublicKey
	poolFees  float64
	netParams *chaincfg.Params
	dbFile    string
}

var cfg Config

// Database with stubbed methods
var db *database.VspDatabase

var nodeConnection *wsrpc.Client

func initConfig() (*Config, error) {
	homePath := "~/.dcrvsp"

	seedPath := filepath.Join(homePath, "sign.seed")
	seed, err := ioutil.ReadFile(seedPath)
	var signKey ed25519.PrivateKey
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.New("seedPath does not exist")
		}

		_, signKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate signing key: %v", err)
		}
		err = ioutil.WriteFile(seedPath, signKey.Seed(), 0400)
		if err != nil {
			return nil, fmt.Errorf("failed to save signing key: %v", err)
		}
	} else {
		signKey = ed25519.NewKeyFromSeed(seed)
	}

	pubKey, ok := signKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to cast signing key: %T", pubKey)
	}

	return &Config{
		netParams: chaincfg.TestNet3Params(),
		dbFile:    filepath.Join(homePath, "database.db"),
		pubKey:    pubKey,
		poolFees:  0.1,
		signKey:   signKey,
	}, nil
}

func main() {
	cfg, err := initConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err = database.New(cfg.dbFile)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}

	defer db.Close()

	// Start HTTP server
	log.Printf("Listening on %s", listen)
	log.Print(newRouter().Run(listen))
}
