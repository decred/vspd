// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package config

import (
	"github.com/decred/dcrd/chaincfg/v3"
)

type Network struct {
	*chaincfg.Params
	DcrdRPCServerPort   string
	WalletRPCServerPort string
	BlockExplorerURL    string
	// MinWallets is the minimum number of voting wallets required for a vspd
	// deployment on this network. vspd will log an error and refuse to start if
	// fewer wallets are configured.
	MinWallets int
	// DCP0005Height is the activation height of DCP-0005 block header
	// commitments agenda on this network.
	DCP0005Height int64
}

var MainNet = Network{
	Params:              chaincfg.MainNetParams(),
	DcrdRPCServerPort:   "9109",
	WalletRPCServerPort: "9110",
	BlockExplorerURL:    "https://dcrdata.decred.org",
	MinWallets:          3,
	// DCP0005Height on mainnet is block
	// 000000000000000010815bed2c4dc431c34a859f4fc70774223dde788e95a01e.
	DCP0005Height: 431488,
}

var TestNet3 = Network{
	Params:              chaincfg.TestNet3Params(),
	DcrdRPCServerPort:   "19109",
	WalletRPCServerPort: "19110",
	BlockExplorerURL:    "https://testnet.dcrdata.org",
	MinWallets:          1,
	// DCP0005Height on testnet3 is block
	// 0000003e54421d585f4a609393a8694509af98f62b8449f245b09fe1389f8f77.
	DCP0005Height: 323328,
}

var SimNet = Network{
	Params:              chaincfg.SimNetParams(),
	DcrdRPCServerPort:   "19556",
	WalletRPCServerPort: "19557",
	BlockExplorerURL:    "...",
	MinWallets:          1,
	// DCP0005Height on simnet is 1 because the agenda will always be active.
	DCP0005Height: 1,
}

// DCP5Active returns true if the DCP-0005 block header commitments agenda is
// active on this network at the provided height, otherwise false.
func (n *Network) DCP5Active(height int64) bool {
	return height >= n.DCP0005Height
}

// CurrentVoteVersion returns the most recent version in the current networks
// consensus agenda deployments.
func (n *Network) CurrentVoteVersion() uint32 {
	var latestVersion uint32
	for version := range n.Deployments {
		if latestVersion < version {
			latestVersion = version
		}
	}
	return latestVersion
}
