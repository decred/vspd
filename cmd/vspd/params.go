// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/decred/dcrd/chaincfg/v3"
)

type netParams struct {
	*chaincfg.Params
	dcrdRPCServerPort   string
	walletRPCServerPort string
	blockExplorerURL    string
	// minWallets is the minimum number of voting wallets required for a vspd
	// deployment on this network. vspd will log an error and refuse to start if
	// fewer wallets are configured.
	minWallets int
}

var mainNetParams = netParams{
	Params:              chaincfg.MainNetParams(),
	dcrdRPCServerPort:   "9109",
	walletRPCServerPort: "9110",
	blockExplorerURL:    "https://dcrdata.decred.org",
	minWallets:          3,
}

var testNet3Params = netParams{
	Params:              chaincfg.TestNet3Params(),
	dcrdRPCServerPort:   "19109",
	walletRPCServerPort: "19110",
	blockExplorerURL:    "https://testnet.dcrdata.org",
	minWallets:          1,
}

var simNetParams = netParams{
	Params:              chaincfg.SimNetParams(),
	dcrdRPCServerPort:   "19556",
	walletRPCServerPort: "19557",
	blockExplorerURL:    "...",
	minWallets:          1,
}
