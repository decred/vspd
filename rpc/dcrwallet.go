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
func (w *WalletConnect) Clients(ctx context.Context, netParams *chaincfg.Params) ([]*WalletRPC, []string) {
	walletClients := make([]*WalletRPC, 0)
	failedConnections := make([]string, 0)

	for _, connect := range []*client(*w) {

		c, newConnection, err := connect.dial(ctx)
		if err != nil {
			log.Errorf("dcrwallet connection error: %v", err)
			failedConnections = append(failedConnections, connect.addr)
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
			log.Errorf("dcrwallet.Version error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}
		walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
		if !exists {
			log.Errorf("dcrwallet.Version response missing 'dcrwalletjsonrpcapi' (wallet=%s)",
				c.String())
			failedConnections = append(failedConnections, connect.addr)
			continue
		}
		if walletVersion.VersionString != requiredWalletVersion {
			log.Errorf("dcrwallet has wrong RPC version (wallet=%s): got %s, expected %s",
				c.String(), walletVersion.VersionString, requiredWalletVersion)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}

		// Verify dcrwallet is on the correct network.
		var netID wire.CurrencyNet
		err = c.Call(ctx, "getcurrentnet", &netID)
		if err != nil {
			log.Errorf("dcrwallet.GetCurrentNet error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}
		if netID != netParams.Net {
			log.Errorf("dcrwallet on wrong network (wallet=%s): running on %s, expected %s",
				c.String(), netID, netParams.Net)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}

		// Verify dcrwallet is voting and unlocked.
		walletRPC := &WalletRPC{c, ctx}
		walletInfo, err := walletRPC.WalletInfo()
		if err != nil {
			log.Errorf("dcrwallet.WalletInfo error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}

		if !walletInfo.Voting {
			// All wallet RPCs can still be used if voting is disabled, so just
			// log an error here. Don't count this as a failed connection.
			log.Errorf("wallet is not voting (wallet=%s)", c.String())
		}
		if !walletInfo.Unlocked {
			// SetVoteChoice can still be used even if the wallet is locked, so
			// just log an error here. Don't count this as a failed connection.
			log.Errorf("wallet is not unlocked (wallet=%s)", c.String())
		}

		walletClients = append(walletClients, walletRPC)

	}

	return walletClients, failedConnections
}

func (c *WalletRPC) WalletInfo() (*wallettypes.WalletInfoResult, error) {
	var walletInfo wallettypes.WalletInfoResult
	err := c.Call(c.ctx, "walletinfo", &walletInfo)
	if err != nil {
		return nil, err
	}
	return &walletInfo, nil
}

func (c *WalletRPC) AddTicketForVoting(votingWIF, blockHash, txHex string) error {
	label := "imported"
	rescan := false
	scanFrom := 0
	err := c.Call(c.ctx, "importprivkey", nil, votingWIF, label, rescan, scanFrom)
	if err != nil {
		return fmt.Errorf("importprivkey failed: %v", err)
	}

	err = c.Call(c.ctx, "addtransaction", nil, blockHash, txHex)
	if err != nil {
		return fmt.Errorf("addtransaction failed: %v", err)
	}

	return nil
}

func (c *WalletRPC) SetVoteChoice(agenda, choice, ticketHash string) error {
	return c.Call(c.ctx, "setvotechoice", nil, agenda, choice, ticketHash)
}
