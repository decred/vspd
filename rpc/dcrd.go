// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
	"github.com/jrick/bitset"
	"github.com/jrick/wsrpc/v2"
)

var (
	requiredDcrdVersion = semver{Major: 8, Minor: 0, Patch: 0}
)

const (
	// These numerical error codes are defined in dcrd/dcrjson. Copied here so
	// we dont need to import the whole package.
	ErrRPCDuplicateTx = -40
	ErrNoTxInfo       = -5
	// This error string is defined in dcrd/internal/mempool. Copied here
	// because it is not exported.
	ErrUnknownOutputs = "references outputs of unknown or fully-spent transaction"
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
	log    slog.Logger
}

func SetupDcrd(user, pass, addr string, cert []byte, params *chaincfg.Params, log slog.Logger,
	blockConnectedChan chan *wire.BlockHeader) DcrdConnect {
	client := setup(user, pass, addr, cert, log)

	client.notifier = &blockConnectedHandler{
		blockConnected: blockConnectedChan,
		log:            log,
	}

	return DcrdConnect{
		client: client,
		params: params,
		log:    log,
	}
}

func (d *DcrdConnect) Close() {
	d.client.Close()
	d.log.Debug("dcrd client closed")
}

// Client creates a new DcrdRPC client instance. Returns an error if dialing
// dcrd fails or if dcrd is misconfigured.
func (d *DcrdConnect) Client() (*DcrdRPC, string, error) {
	ctx := context.TODO()
	c, newConnection, err := d.client.dial(ctx)
	if err != nil {
		return nil, d.client.addr, fmt.Errorf("dcrd dial error: %w", err)
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

	// Request blockconnected notifications.
	if d.client.notifier != nil {
		err = c.Call(ctx, "notifyblocks", nil)
		if err != nil {
			return nil, d.client.addr, fmt.Errorf("notifyblocks failed: %w", err)
		}
	}

	d.log.Debugf("Connected to dcrd")

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

// DecodeRawTransaction uses decoderawtransaction RPC to decode raw transaction bytes.
func (c *DcrdRPC) DecodeRawTransaction(txHex string) (*dcrdtypes.TxRawDecodeResult, error) {
	var resp dcrdtypes.TxRawDecodeResult
	err := c.Call(c.ctx, "decoderawtransaction", &resp, txHex)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// SendRawTransaction uses sendrawtransaction RPC to broadcast a transaction to
// the network. It ignores errors caused by duplicate transactions.
func (c *DcrdRPC) SendRawTransaction(txHex string) error {
	const allowHighFees = false
	err := c.Call(c.ctx, "sendrawtransaction", nil, txHex, allowHighFees)
	if err != nil {

		// Ignore errors caused by the transaction already existing in the
		// mempool or in a mined block.

		// Error code -40 (ErrRPCDuplicateTx) is completely ignorable because it
		// indicates that dcrd definitely already has this transaction.
		var e *wsrpc.Error
		if errors.As(err, &e) && e.Code == ErrRPCDuplicateTx {
			return nil
		}

		// Errors about orphan/spent outputs indicate that dcrd *might* already
		// have this transaction. Use getrawtransaction to confirm.
		if strings.Contains(err.Error(), ErrUnknownOutputs) {
			_, getErr := c.GetRawTransaction(txHex)
			if getErr == nil {
				return nil
			}
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

	return agenda.Status == dcrdtypes.AgendaInfoStatusActive, nil
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

	blockHeader, err := c.GetBlockHeaderVerbose(bestBlockHash)
	if err != nil {
		return nil, err
	}
	return blockHeader, nil
}

// GetBlockHeaderVerbose uses getblockheader RPC with verbose=true to retrieve
// the header of the requested block.
func (c *DcrdRPC) GetBlockHeaderVerbose(blockHash string) (*dcrdtypes.GetBlockHeaderVerboseResult, error) {
	const verbose = true
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err := c.Call(c.ctx, "getblockheader", &blockHeader, blockHash, verbose)
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
