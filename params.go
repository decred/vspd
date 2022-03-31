// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/decred/dcrd/chaincfg/v3"
)

type netParams struct {
	*chaincfg.Params
	DcrdRPCServerPort   string
	WalletRPCServerPort string
	BlockExplorerURL    string
	// MinWallets is the minimum number of voting wallets required for a vspd
	// deployment on this network. vspd will log an error and refuse to start if
	// fewer wallets are configured.
	MinWallets int
}

var mainNetParams = netParams{
	Params:              chaincfg.MainNetParams(),
	DcrdRPCServerPort:   "9109",
	WalletRPCServerPort: "9110",
	BlockExplorerURL:    "https://dcrdata.decred.org",
	MinWallets:          3,
}

var testNet3Params = netParams{
	Params:              chaincfg.TestNet3Params(),
	DcrdRPCServerPort:   "19109",
	WalletRPCServerPort: "19110",
	BlockExplorerURL:    "https://testnet.dcrdata.org",
	MinWallets:          1,
}

var simNetParams = netParams{
	Params:              chaincfg.SimNetParams(),
	DcrdRPCServerPort:   "19556",
	WalletRPCServerPort: "19557",
	BlockExplorerURL:    "...",
	MinWallets:          1,
}
