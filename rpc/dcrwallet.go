package rpc

import (
	"context"
	"fmt"

	wallettypes "decred.org/dcrwallet/rpc/jsonrpc/types"
	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

const (
	requiredWalletVersion = "8.1.0"
)

// WalletRPC provides methods for calling dcrwallet JSON-RPCs without exposing the details
// of JSON encoding.
type WalletRPC struct {
	Caller
	ctx context.Context
}

// WalletClient creates a new WalletRPC client instance from a caller.
func WalletClient(ctx context.Context, c Caller, netParams *chaincfg.Params) (*WalletRPC, error) {

	// Verify dcrwallet is at the required api version.
	var verMap map[string]dcrdtypes.VersionResult
	err := c.Call(ctx, "version", &verMap)
	if err != nil {
		return nil, fmt.Errorf("version check failed: %v", err)
	}
	walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
	if !exists {
		return nil, fmt.Errorf("version response missing 'dcrwalletjsonrpcapi'")
	}
	if walletVersion.VersionString != requiredWalletVersion {
		return nil, fmt.Errorf("wrong dcrwallet RPC version: got %s, expected %s",
			walletVersion.VersionString, requiredWalletVersion)
	}

	// Verify dcrwallet is voting, unlocked, and is connected to dcrd (not SPV).
	var walletInfo wallettypes.WalletInfoResult
	err = c.Call(ctx, "walletinfo", &walletInfo)
	if err != nil {
		return nil, fmt.Errorf("walletinfo check failed: %v", err)
	}
	if !walletInfo.Voting {
		return nil, fmt.Errorf("wallet has voting disabled")
	}
	if !walletInfo.Unlocked {
		return nil, fmt.Errorf("wallet is not unlocked")
	}
	if !walletInfo.DaemonConnected {
		return nil, fmt.Errorf("wallet is not connected to dcrd")
	}

	// Verify dcrwallet is on the correct network.
	var netID wire.CurrencyNet
	err = c.Call(ctx, "getcurrentnet", &netID)
	if err != nil {
		return nil, fmt.Errorf("getcurrentnet check failed: %v", err)
	}
	if netID != netParams.Net {
		return nil, fmt.Errorf("dcrwallet running on %s, expected %s", netID, netParams.Net)
	}

	return &WalletRPC{c, ctx}, nil
}

func (c *WalletRPC) AddTransaction(blockHash, txHex string) error {
	return c.Call(c.ctx, "addtransaction", nil, blockHash, txHex)
}

func (c *WalletRPC) ImportPrivKey(votingWIF string) error {
	label := "imported"
	rescan := false
	scanFrom := 0
	return c.Call(c.ctx, "importprivkey", nil, votingWIF, label, rescan, scanFrom)
}

func (c *WalletRPC) SetVoteChoice(agenda, choice, ticketHash string) error {
	return c.Call(c.ctx, "setvotechoice", nil, agenda, choice, ticketHash)
}
