// Copyright (c) 2021-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"fmt"

	wallettypes "decred.org/dcrwallet/v4/rpc/jsonrpc/types"
	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
)

var (
	requiredWalletVersion = semver{Major: 9, Minor: 0, Patch: 0}
)

// WalletRPC provides methods for calling dcrwallet JSON-RPCs without exposing the details
// of JSON encoding.
type WalletRPC struct {
	Caller
}

type WalletConnect struct {
	clients []*client
	params  *chaincfg.Params
	log     slog.Logger
}

func SetupWallet(user, pass, addrs []string, cert [][]byte, params *chaincfg.Params, log slog.Logger) WalletConnect {
	clients := make([]*client, len(addrs))

	for i := range len(addrs) {
		clients[i] = setup(user[i], pass[i], addrs[i], cert[i], log)
	}

	return WalletConnect{
		clients: clients,
		params:  params,
		log:     log,
	}
}

func (w *WalletConnect) Close() {
	for _, client := range w.clients {
		client.Close()
	}
	w.log.Debug("dcrwallet clients closed")
}

// Clients loops over each wallet and tries to establish a connection. It
// increments a count of failed connections if a connection cannot be
// established, or if the wallet is misconfigured.
func (w *WalletConnect) Clients() ([]*WalletRPC, []string) {
	ctx := context.TODO()
	walletClients := make([]*WalletRPC, 0)
	failedConnections := make([]string, 0)

	for _, connect := range w.clients {

		c, newConnection, err := connect.dial(ctx)
		if err != nil {
			w.log.Errorf("dcrwallet dial error: %v", err)
			failedConnections = append(failedConnections, connect.addr)
			continue
		}

		// If this is a reused connection, we don't need to validate the
		// dcrwallet config again.
		if !newConnection {
			walletClients = append(walletClients, &WalletRPC{c})
			continue
		}

		// Verify dcrwallet is at the required api version.
		var verMap map[string]dcrdtypes.VersionResult
		err = c.Call(ctx, "version", &verMap)
		if err != nil {
			w.log.Errorf("dcrwallet.Version error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}

		ver, exists := verMap["dcrwalletjsonrpcapi"]
		if !exists {
			w.log.Errorf("dcrwallet.Version response missing 'dcrwalletjsonrpcapi' (wallet=%s)",
				c.String())
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}

		sVer := semver{ver.Major, ver.Minor, ver.Patch}
		if !semverCompatible(requiredWalletVersion, sVer) {
			w.log.Errorf("dcrwallet has incompatible JSON-RPC version (wallet=%s): got %s, expected %s",
				c.String(), sVer, requiredWalletVersion)
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}

		// Verify dcrwallet is on the correct network.
		var netID wire.CurrencyNet
		err = c.Call(ctx, "getcurrentnet", &netID)
		if err != nil {
			w.log.Errorf("dcrwallet.GetCurrentNet error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}
		if netID != w.params.Net {
			w.log.Errorf("dcrwallet on wrong network (wallet=%s): running on %s, expected %s",
				c.String(), netID, w.params.Net)
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}

		// Verify dcrwallet is voting and unlocked.
		walletRPC := &WalletRPC{c}
		walletInfo, err := walletRPC.WalletInfo()
		if err != nil {
			w.log.Errorf("dcrwallet.WalletInfo error (wallet=%s): %v", c.String(), err)
			failedConnections = append(failedConnections, connect.addr)
			connect.Close()
			continue
		}

		if !walletInfo.ManualTickets {
			// All wallet should not be adding tickets found via the network.  This
			// misconfiguration should not have a negative impact on users, so just
			// log an error here.  Don't count this as a failed connection.
			w.log.Errorf("wallet does not have manual tickets enabled (wallet=%s)", c.String())
		}
		if !walletInfo.Voting {
			// All wallet RPCs can still be used if voting is disabled, so just
			// log an error here. Don't count this as a failed connection.
			w.log.Errorf("wallet is not voting (wallet=%s)", c.String())
		}
		if !walletInfo.Unlocked {
			// SetVoteChoice can still be used even if the wallet is locked, so
			// just log an error here. Don't count this as a failed connection.
			w.log.Errorf("wallet is not unlocked (wallet=%s)", c.String())
		}

		walletClients = append(walletClients, walletRPC)

	}

	return walletClients, failedConnections
}

// WalletInfo uses walletinfo RPC to retrieve information about how the
// dcrwallet instance is configured.
func (c *WalletRPC) WalletInfo() (*wallettypes.WalletInfoResult, error) {
	var walletInfo wallettypes.WalletInfoResult
	err := c.Call(context.TODO(), "walletinfo", &walletInfo)
	if err != nil {
		return nil, err
	}
	return &walletInfo, nil
}

// AddTicketForVoting uses importprivkey RPC, followed by addtransaction RPC, to
// add a new ticket to a voting wallet.
func (c *WalletRPC) AddTicketForVoting(votingWIF, blockHash, txHex string) error {
	const label = "imported"
	const rescan = false
	const scanFrom = 0
	err := c.Call(context.TODO(), "importprivkey", nil, votingWIF, label, rescan, scanFrom)
	if err != nil {
		return fmt.Errorf("importprivkey failed: %w", err)
	}

	err = c.Call(context.TODO(), "addtransaction", nil, blockHash, txHex)
	if err != nil {
		return fmt.Errorf("addtransaction failed: %w", err)
	}

	return nil
}

// SetVoteChoice uses setvotechoice RPC to set the vote choice on the given
// agenda, for the given ticket.
func (c *WalletRPC) SetVoteChoice(agenda, choice, ticketHash string) error {
	return c.Call(context.TODO(), "setvotechoice", nil, agenda, choice, ticketHash)
}

// GetBestBlockHeight uses getblockcount RPC to query the height of the best
// block known by the dcrwallet instance.
func (c *WalletRPC) GetBestBlockHeight() (int64, error) {
	var height int64
	err := c.Call(context.TODO(), "getblockcount", &height)
	if err != nil {
		return 0, err
	}
	return height, nil
}

// TicketInfo uses ticketinfo RPC to retrieve a detailed list of all tickets
// known by this dcrwallet instance.
func (c *WalletRPC) TicketInfo(startHeight int64) (map[string]*wallettypes.TicketInfoResult, error) {
	var result []*wallettypes.TicketInfoResult
	err := c.Call(context.TODO(), "ticketinfo", &result, startHeight)
	if err != nil {
		return nil, err
	}

	// For easier access later on, store the tickets in a map using their hash
	// as the key.
	tickets := make(map[string]*wallettypes.TicketInfoResult, len(result))
	for _, t := range result {
		tickets[t.Hash] = t
	}

	return tickets, err
}

// RescanFrom uses rescanwallet RPC to trigger the wallet to perform a rescan
// from the specified block height.
func (c *WalletRPC) RescanFrom(fromHeight int64) error {
	return c.Call(context.TODO(), "rescanwallet", nil, fromHeight)
}

// SetTreasuryPolicy sets the specified tickets voting policy for all tspends
// published by the given treasury key.
func (c *WalletRPC) SetTreasuryPolicy(key, policy, ticket string) error {
	return c.Call(context.TODO(), "settreasurypolicy", nil, key, policy, ticket)
}

// SetTSpendPolicy sets the specified tickets voting policy for a single tspend
// identified by its hash.
func (c *WalletRPC) SetTSpendPolicy(tSpend, policy, ticket string) error {
	return c.Call(context.TODO(), "settspendpolicy", nil, tSpend, policy, ticket)
}
