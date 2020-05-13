package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/rpcclient"
)

const listen = ":3000"

// Config vars
var (
	signKey   ed25519.PrivateKey
	pubKey    ed25519.PublicKey
	poolFees  float64
	netParams *chaincfg.Params
)

// Database with stubbed methods
var db Database

var nodeConnection *rpcclient.Client
var walletConnection *WalletClient

func initConfig() {
	seedPath := filepath.Join("dcrvsp", "sign.seed")
	seed, err := ioutil.ReadFile(seedPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Fatal("seedPath does not exist")
		}

		_, signKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatalf("failed to generate signing key: %v", err)
		}
		err = ioutil.WriteFile(seedPath, signKey.Seed(), 0400)
		if err != nil {
			log.Fatalf("failed to save signing key: %v", err)
		}
	} else {
		signKey = ed25519.NewKeyFromSeed(seed)
	}

	pubKey, ok := signKey.Public().(ed25519.PublicKey)
	if !ok {
		log.Fatalf("failed to cast signing key: %T", pubKey)
	}

	netParams = chaincfg.TestNet3Params()
}

func main() {

	initConfig()

	// Start HTTP server
	log.Printf("Listening on %s", listen)
	log.Print(newRouter().Run(listen))
}
