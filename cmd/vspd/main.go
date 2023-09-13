// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/decred/vspd/internal/version"
)

func main() {
	os.Exit(run())
}

// run is the real main function for vspd. It is necessary to work around the
// fact that deferred functions do not run when os.Exit() is called.
func run() int {
	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadConfig error: %v\n", err)
		return 1
	}

	log := cfg.logger("VSP")

	defer log.Criticalf("Shutdown complete")
	log.Criticalf("Version %s (Go version %s %s/%s)", version.String(),
		runtime.Version(), runtime.GOOS, runtime.GOARCH)

	if cfg.netParams == &mainNetParams && version.IsPreRelease() {
		log.Warnf("")
		log.Warnf("\tWARNING: This is a pre-release version of vspd which should not be used on mainnet.")
		log.Warnf("")
	}

	if cfg.VspClosed {
		log.Warnf("")
		log.Warnf("\tWARNING: Config --vspclosed is set. This will prevent vspd from accepting new tickets.")
		log.Warnf("")
	}

	vspd, err := newVspd(cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "newVspd error: %v\n", err)
		return 1
	}

	return vspd.run()
}
