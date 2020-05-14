package main

import (
	"log"
	"os"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jrick/wsrpc/v2"
)

var cfg *config

var db *database.VspDatabase

var nodeConnection *wsrpc.Client

func main() {
	var err error
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	db, err = database.New(cfg.dbPath)
	if err != nil {
		log.Fatalf("database error: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Start HTTP server
	log.Printf("Listening on %s", cfg.Listen)
	log.Print(newRouter().Run(cfg.Listen))
}
