package main

import (
	"github.com/decred/dcrd/chaincfg/v3"
)

type netParams struct {
	*chaincfg.Params
	WalletRPCServerPort string
}

var mainNetParams = netParams{
	Params:              chaincfg.MainNetParams(),
	WalletRPCServerPort: "9110",
}

var testNet3Params = netParams{
	Params:              chaincfg.TestNet3Params(),
	WalletRPCServerPort: "19110",
}

var simNetParams = netParams{
	Params:              chaincfg.SimNetParams(),
	WalletRPCServerPort: "19557",
}
