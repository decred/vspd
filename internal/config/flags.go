// Copyright (c) 2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"github.com/jessevdk/go-flags"
)

// WriteHelp will write the application help to stdout if it has been requested
// via command line options. Only the help option is evaluated, any invalid
// options are ignored. The return value indicates whether the help message was
// printed or not.
func WriteHelp(cfg interface{}) bool {
	helpOpts := flags.Options(flags.HelpFlag | flags.PrintErrors | flags.IgnoreUnknown)
	_, err := flags.NewParser(cfg, helpOpts).Parse()
	return flags.WroteHelp(err)
}
