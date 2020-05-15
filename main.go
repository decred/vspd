package main

import (
	"fmt"
	"os"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jrick/wsrpc/v2"
)

var (
	cfg            *config
	db             *database.VspDatabase
	nodeConnection *wsrpc.Client
)

func main() {
	var err error
	cfg, err = loadConfig()
	if err != nil {
		// Don't use logger here because it may not be initialised yet.
		fmt.Fprintf(os.Stderr, "config error: %v", err)
		os.Exit(1)
	}

	db, err = database.New(cfg.dbPath)
	if err != nil {
		log.Errorf("database error: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	// Start HTTP server
	log.Infof("Listening on %s", cfg.Listen)
	// TODO: Make releaseMode properly configurable.
	releaseMode := false
	err = newRouter(releaseMode).Run(cfg.Listen)
	if err != nil {
		log.Errorf("web server terminated with error: %v", err)
		os.Exit(1)
	}
}
