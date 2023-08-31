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
	// dcp0005Height is the activation height of DCP-0005 block header
	// commitments agenda on this network.
	dcp0005Height int64
}

var mainNetParams = netParams{
	Params:              chaincfg.MainNetParams(),
	dcrdRPCServerPort:   "9109",
	walletRPCServerPort: "9110",
	blockExplorerURL:    "https://dcrdata.decred.org",
	minWallets:          3,
	// dcp0005Height on mainnet is block
	// 000000000000000010815bed2c4dc431c34a859f4fc70774223dde788e95a01e.
	dcp0005Height: 431488,
}

var testNet3Params = netParams{
	Params:              chaincfg.TestNet3Params(),
	dcrdRPCServerPort:   "19109",
	walletRPCServerPort: "19110",
	blockExplorerURL:    "https://testnet.dcrdata.org",
	minWallets:          1,
	// dcp0005Height on testnet3 is block
	// 0000003e54421d585f4a609393a8694509af98f62b8449f245b09fe1389f8f77.
	dcp0005Height: 323328,
}

var simNetParams = netParams{
	Params:              chaincfg.SimNetParams(),
	dcrdRPCServerPort:   "19556",
	walletRPCServerPort: "19557",
	blockExplorerURL:    "...",
	minWallets:          1,
	// dcp0005Height on simnet is 1 because the agenda will always be active.
	dcp0005Height: 1,
}

// dcp5Active returns true if the DCP-0005 block header commitments agenda is
// active on this network at the provided height, otherwise false.
func (n *netParams) dcp5Active(height int64) bool {
	return height >= n.dcp0005Height
}
