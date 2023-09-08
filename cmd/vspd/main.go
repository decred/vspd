// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
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

	vspd, err := newVspd(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "newVspd error: %v\n", err)
		return 1
	}

	return vspd.run()
}
