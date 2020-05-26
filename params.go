package main

import (
	"github.com/decred/dcrd/chaincfg/v3"
)

type netParams struct {
	*chaincfg.Params
	DcrdRPCServerPort   string
	WalletRPCServerPort string
}

var mainNetParams = netParams{
	Params:              chaincfg.MainNetParams(),
	DcrdRPCServerPort:   "9109",
	WalletRPCServerPort: "9110",
}

var testNet3Params = netParams{
	Params:              chaincfg.TestNet3Params(),
	DcrdRPCServerPort:   "19109",
	WalletRPCServerPort: "19110",
}

var simNetParams = netParams{
	Params:              chaincfg.SimNetParams(),
	DcrdRPCServerPort:   "19556",
	WalletRPCServerPort: "19557",
}
