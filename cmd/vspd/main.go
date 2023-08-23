// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
)

func main() {
	// Load config file and parse CLI args.
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "loadConfig error: %v\n", err)
		os.Exit(1)
	}

	vspd, err := newVspd(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "newVspd error: %v\n", err)
		os.Exit(1)
	}

	// Run until an exit code is returned.
	os.Exit(vspd.run())
}
