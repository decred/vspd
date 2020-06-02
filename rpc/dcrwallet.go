package rpc

import (
	"context"
	"fmt"
	"sync"

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

type WalletConnect []connect

func SetupWallet(ctx context.Context, shutdownWg *sync.WaitGroup, user, pass string, addrs []string, cert []byte) WalletConnect {
	walletConnect := make(WalletConnect, len(addrs))

	for i := 0; i < len(addrs); i++ {
		walletConnect[i] = setup(ctx, shutdownWg, user, pass,
			addrs[i], cert, nil)
	}

	return walletConnect
}

// Clients creates an array of new WalletRPC client instances. Returns an error
// if dialing any wallet fails, or if any wallet is misconfigured.
func (w *WalletConnect) Clients(ctx context.Context, netParams *chaincfg.Params) ([]*WalletRPC, error) {
	walletClients := make([]*WalletRPC, len(*w))

	for i := 0; i < len(*w); i++ {

		c, newConnection, err := []connect(*w)[i]()
		if err != nil {
			return nil, fmt.Errorf("dcrwallet connection error: %v", err)
		}

		// If this is a reused connection, we don't need to validate the
		// dcrwallet config again.
		if !newConnection {
			walletClients[i] = &WalletRPC{c, ctx}
			continue
		}

		// Verify dcrwallet is at the required api version.
		var verMap map[string]dcrdtypes.VersionResult
		err = c.Call(ctx, "version", &verMap)
		if err != nil {
			return nil, fmt.Errorf("version check on dcrwallet '%s' failed: %v",
				c.String(), err)
		}
		walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
		if !exists {
			return nil, fmt.Errorf("version response on dcrwallet '%s' missing 'dcrwalletjsonrpcapi'",
				c.String())
		}
		if walletVersion.VersionString != requiredWalletVersion {
			return nil, fmt.Errorf("dcrwallet '%s' has wrong RPC version: got %s, expected %s",
				c.String(), walletVersion.VersionString, requiredWalletVersion)
		}

		// Verify dcrwallet is voting, unlocked, and is connected to dcrd (not SPV).
		var walletInfo wallettypes.WalletInfoResult
		err = c.Call(ctx, "walletinfo", &walletInfo)
		if err != nil {
			return nil, fmt.Errorf("walletinfo check on dcrwallet '%s' failed: %v",
				c.String(), err)
		}

		// TODO: The following 3 checks should probably just log a warning/error and
		// not return.
		// addtransaction and setvotechoice can still be used with a locked wallet.
		// importprivkey will fail if wallet is locked.

		if !walletInfo.Voting {
			return nil, fmt.Errorf("wallet '%s' has voting disabled", c.String())
		}
		if !walletInfo.Unlocked {
			return nil, fmt.Errorf("wallet '%s' is not unlocked", c.String())
		}
		if !walletInfo.DaemonConnected {
			return nil, fmt.Errorf("wallet '%s' is not connected to dcrd", c.String())
		}

		// Verify dcrwallet is on the correct network.
		var netID wire.CurrencyNet
		err = c.Call(ctx, "getcurrentnet", &netID)
		if err != nil {
			return nil, fmt.Errorf("getcurrentnet check on dcrwallet '%s' failed: %v",
				c.String(), err)
		}
		if netID != netParams.Net {
			return nil, fmt.Errorf("dcrwallet '%s' running on %s, expected %s",
				c.String(), netID, netParams.Net)
		}

		walletClients[i] = &WalletRPC{c, ctx}

	}

	return walletClients, nil
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
