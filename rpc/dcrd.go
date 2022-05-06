// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/v4"
	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/wire"
	"github.com/jrick/bitset"
	"github.com/jrick/wsrpc/v2"
)

var (
	requiredDcrdVersion = semver{Major: 7, Minor: 0, Patch: 0}

	activeStatus = blockchain.ThresholdStateTuple{State: blockchain.ThresholdActive}.String()
)

// These error codes are defined in dcrd/dcrjson. They are copied here so we
// dont need to import the whole package.
const (
	ErrRPCDuplicateTx = -40
	ErrNoTxInfo       = -5
)

// DcrdRPC provides methods for calling dcrd JSON-RPCs without exposing the details
// of JSON encoding.
type DcrdRPC struct {
	Caller
	ctx context.Context
}

type DcrdConnect struct {
	client *client
	params *chaincfg.Params
}

func SetupDcrd(user, pass, addr string, cert []byte, n wsrpc.Notifier, params *chaincfg.Params) DcrdConnect {
	return DcrdConnect{
		client: setup(user, pass, addr, cert, n),
		params: params,
	}
}

func (d *DcrdConnect) Close() {
	d.client.Close()
}

// Client creates a new DcrdRPC client instance. Returns an error if dialing
// dcrd fails or if dcrd is misconfigured.
func (d *DcrdConnect) Client(ctx context.Context) (*DcrdRPC, string, error) {
	c, newConnection, err := d.client.dial(ctx)
	if err != nil {
		return nil, d.client.addr, fmt.Errorf("dcrd connection error: %w", err)
	}

	// If this is a reused connection, we don't need to validate the dcrd config
	// again.
	if !newConnection {
		return &DcrdRPC{c, ctx}, d.client.addr, nil
	}

	// Verify dcrd is at the required api version.
	var verMap map[string]dcrdtypes.VersionResult
	err = c.Call(ctx, "version", &verMap)
	if err != nil {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd version check failed: %w", err)
	}

	ver, exists := verMap["dcrdjsonrpcapi"]
	if !exists {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd version response missing 'dcrdjsonrpcapi'")
	}

	sVer := semver{ver.Major, ver.Minor, ver.Patch}
	if !semverCompatible(requiredDcrdVersion, sVer) {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd has incompatible JSON-RPC version: got %s, expected %s",
			sVer, requiredDcrdVersion)
	}

	// Verify dcrd is on the correct network.
	var netID wire.CurrencyNet
	err = c.Call(ctx, "getcurrentnet", &netID)
	if err != nil {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd getcurrentnet check failed: %w", err)
	}
	if netID != d.params.Net {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd running on %s, expected %s", netID, d.params.Net)
	}

	// Verify dcrd has tx index enabled (required for getrawtransaction).
	var info dcrdtypes.InfoChainResult
	err = c.Call(ctx, "getinfo", &info)
	if err != nil {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd getinfo check failed: %w", err)
	}
	if !info.TxIndex {
		d.client.Close()
		return nil, d.client.addr, errors.New("dcrd does not have transaction index enabled (--txindex)")
	}

	return &DcrdRPC{c, ctx}, d.client.addr, nil
}

// GetRawTransaction uses getrawtransaction RPC to retrieve details about the
// transaction with the provided hash.
func (c *DcrdRPC) GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error) {
	verbose := 1
	var resp dcrdtypes.TxRawResult
	err := c.Call(c.ctx, "getrawtransaction", &resp, txHash, verbose)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendRawTransaction uses sendrawtransaction RPC to broadcast a transaction to
// the network. It ignores errors caused by duplicate transactions.
func (c *DcrdRPC) SendRawTransaction(txHex string) error {
	allowHighFees := false
	err := c.Call(c.ctx, "sendrawtransaction", nil, txHex, allowHighFees)
	if err != nil {
		// sendrawtransaction returns error code -40 (ErrRPCDuplicateTx) if the
		// provided transaction already exists in the mempool or in a mined
		// block.
		// It's not a problem if the transaction has already been broadcast, so
		// we will capture this error and return nil.
		var e *wsrpc.Error
		if errors.As(err, &e) && e.Code == ErrRPCDuplicateTx {
			return nil
		}

		return err
	}
	return nil
}

// IsDCP0010Active uses getblockchaininfo RPC to determine if the DCP-0010
// agenda has activated on the current network.
func (c *DcrdRPC) IsDCP0010Active() (bool, error) {
	var info dcrdtypes.GetBlockChainInfoResult
	err := c.Call(c.ctx, "getblockchaininfo", &info)
	if err != nil {
		return false, err
	}

	agenda, ok := info.Deployments[chaincfg.VoteIDChangeSubsidySplit]
	if !ok {
		return false, fmt.Errorf("getblockchaininfo did not return agenda %q",
			chaincfg.VoteIDChangeSubsidySplit)
	}

	return agenda.Status == activeStatus, nil
}

// NotifyBlocks uses notifyblocks RPC to request new block notifications from dcrd.
func (c *DcrdRPC) NotifyBlocks() error {
	return c.Call(c.ctx, "notifyblocks", nil)
}

// GetBestBlockHeader uses getbestblockhash RPC, followed by getblockheader RPC,
// to retrieve the header of the best block known to the dcrd instance.
func (c *DcrdRPC) GetBestBlockHeader() (*dcrdtypes.GetBlockHeaderVerboseResult, error) {
	var bestBlockHash string
	err := c.Call(c.ctx, "getbestblockhash", &bestBlockHash)
	if err != nil {
		return nil, err
	}

	verbose := true
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err = c.Call(c.ctx, "getblockheader", &blockHeader, bestBlockHash, verbose)
	if err != nil {
		return nil, err
	}
	return &blockHeader, nil
}

// ExistsLiveTicket uses existslivetickets RPC to check if the provided ticket
// hash is a live ticket known to the dcrd instance.
func (c *DcrdRPC) ExistsLiveTicket(ticketHash string) (bool, error) {
	var exists string
	err := c.Call(c.ctx, "existslivetickets", &exists, []string{ticketHash})
	if err != nil {
		return false, err
	}

	existsBytes := make([]byte, hex.DecodedLen(len(exists)))
	_, err = hex.Decode(existsBytes, []byte(exists))
	if err != nil {
		return false, err
	}

	return bitset.Bytes(existsBytes).Get(0), nil
}

// ParseBlockConnectedNotification extracts the block header from a
// blockconnected JSON-RPC notification.
func ParseBlockConnectedNotification(params json.RawMessage) (*wire.BlockHeader, error) {
	var notif []string
	err := json.Unmarshal(params, &notif)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}

	if len(notif) == 0 {
		return nil, errors.New("notification is empty")
	}

	rawHeader := notif[0]
	var header wire.BlockHeader
	err = header.Deserialize(hex.NewDecoder(bytes.NewReader([]byte(rawHeader))))
	if err != nil {
		return nil, fmt.Errorf("error creating block header from bytes: %w", err)
	}

	return &header, nil
}
