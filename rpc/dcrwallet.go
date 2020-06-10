package rpc

import (
	"context"

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

type WalletConnect []*client

func SetupWallet(user, pass string, addrs []string, cert []byte) WalletConnect {
	walletConnect := make(WalletConnect, len(addrs))

	for i := 0; i < len(addrs); i++ {
		walletConnect[i] = setup(user, pass, addrs[i], cert, nil)
	}

	return walletConnect
}

func (w *WalletConnect) Close() {
	for _, connect := range []*client(*w) {
		connect.Close()
	}
}

// Clients loops over each wallet and tries to establish a connection. It
// increments a count of failed connections if a connection cannot be
// established, or if the wallet is misconfigured.
func (w *WalletConnect) Clients(ctx context.Context, netParams *chaincfg.Params) ([]*WalletRPC, int) {
	walletClients := make([]*WalletRPC, 0)
	failedConnections := 0

	for _, connect := range []*client(*w) {

		c, newConnection, err := connect.dial(ctx)
		if err != nil {
			log.Errorf("dcrwallet connection error: %v", err)
			failedConnections++
			continue
		}

		// If this is a reused connection, we don't need to validate the
		// dcrwallet config again.
		if !newConnection {
			walletClients = append(walletClients, &WalletRPC{c, ctx})
			continue
		}

		// Verify dcrwallet is at the required api version.
		var verMap map[string]dcrdtypes.VersionResult
		err = c.Call(ctx, "version", &verMap)
		if err != nil {
			log.Errorf("version check on dcrwallet '%s' failed: %v", c.String(), err)
			failedConnections++
			continue
		}
		walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
		if !exists {
			log.Errorf("version response on dcrwallet '%s' missing 'dcrwalletjsonrpcapi'",
				c.String())
			failedConnections++
			continue
		}
		if walletVersion.VersionString != requiredWalletVersion {
			log.Errorf("dcrwallet '%s' has wrong RPC version: got %s, expected %s",
				c.String(), walletVersion.VersionString, requiredWalletVersion)
			failedConnections++
			continue
		}

		// Verify dcrwallet is voting and unlocked.
		var walletInfo wallettypes.WalletInfoResult
		err = c.Call(ctx, "walletinfo", &walletInfo)
		if err != nil {
			log.Errorf("walletinfo check on dcrwallet '%s' failed: %v", c.String(), err)
			failedConnections++
			continue
		}

		if !walletInfo.Voting {
			// All wallet RPCs can still be used if voting is disabled, so just
			// log an error here. Don't count this as a failed connection.
			log.Errorf("wallet '%s' has voting disabled", c.String())
		}
		if !walletInfo.Unlocked {
			// If wallet is locked, ImportPrivKey cannot be used.
			log.Errorf("wallet '%s' is not unlocked", c.String())
			failedConnections++
			continue
		}

		// Verify dcrwallet is on the correct network.
		var netID wire.CurrencyNet
		err = c.Call(ctx, "getcurrentnet", &netID)
		if err != nil {
			log.Errorf("getcurrentnet check on dcrwallet '%s' failed: %v", c.String(), err)
			failedConnections++
			continue
		}
		if netID != netParams.Net {
			log.Errorf("dcrwallet '%s' running on %s, expected %s", c.String(), netID, netParams.Net)
			failedConnections++
			continue
		}

		walletClients = append(walletClients, &WalletRPC{c, ctx})

	}

	return walletClients, failedConnections
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
