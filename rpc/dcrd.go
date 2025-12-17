// Copyright (c) 2021-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/dcrd/blockchain/standalone/v2"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/gcs/v4"
	"github.com/decred/dcrd/gcs/v4/blockcf2"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/wire"
	"github.com/decred/slog"
	"github.com/jrick/bitset"
	"github.com/jrick/wsrpc/v2"
)

var (
	requiredDcrdVersion = semver{Major: 8, Minor: 3, Patch: 0}
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
	c, newConnection, err := d.client.dial(context.TODO())
	if err != nil {
		return nil, d.client.addr, fmt.Errorf("dcrd dial error: %w", err)
	}

	dcrdRPC := &DcrdRPC{c}

	// If this is a reused connection, we don't need to validate the dcrd config
	// again.
	if !newConnection {
		return dcrdRPC, d.client.addr, nil
	}

	// Verify dcrd is at the required api version.
	ver, err := dcrdRPC.version()
	if err != nil {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd version check failed: %w", err)
	}

	if !semverCompatible(requiredDcrdVersion, *ver) {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd has incompatible JSON-RPC version: got %s, expected %s",
			ver, requiredDcrdVersion)
	}

	// Verify dcrd is on the correct network.
	netID, err := dcrdRPC.getCurrentNet()
	if err != nil {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd getcurrentnet check failed: %w", err)
	}
	if netID != d.params.Net {
		d.client.Close()
		return nil, d.client.addr, fmt.Errorf("dcrd running on %s, expected %s", netID, d.params.Net)
	}

	// Verify dcrd has tx index enabled (required for getrawtransaction).
	info, err := dcrdRPC.getInfo()
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
		err = dcrdRPC.NotifyBlocks()
		if err != nil {
			return nil, d.client.addr, fmt.Errorf("notifyblocks failed: %w", err)
		}
	}

	d.log.Debugf("Connected to dcrd")

	return &DcrdRPC{c}, d.client.addr, nil
}

// version uses version RPC to retrieve the API version of dcrd.
func (c *DcrdRPC) version() (*semver, error) {
	var verMap map[string]dcrdtypes.VersionResult
	err := c.Call(context.TODO(), "version", &verMap)
	if err != nil {
		return nil, err
	}

	if ver, ok := verMap["dcrdjsonrpcapi"]; ok {
		return &semver{ver.Major, ver.Minor, ver.Patch}, nil
	}

	return nil, fmt.Errorf("response missing %q", "dcrdjsonrpcapi")
}

// getCurrentNet uses getcurrentnet RPC to return the Decred network the wallet
// is connected to.
func (c *DcrdRPC) getCurrentNet() (wire.CurrencyNet, error) {
	var netID wire.CurrencyNet
	err := c.Call(context.TODO(), "getcurrentnet", &netID)
	if err != nil {
		return 0, err
	}
	return netID, nil
}

// getInfo uses getinfo RPC to return various daemon, network, and chain info.
func (c *DcrdRPC) getInfo() (*dcrdtypes.InfoChainResult, error) {
	var info dcrdtypes.InfoChainResult
	err := c.Call(context.TODO(), "getinfo", &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetRawTransaction uses getrawtransaction RPC to retrieve details about the
// transaction with the provided hash.
func (c *DcrdRPC) GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error) {
	verbose := 1
	var resp dcrdtypes.TxRawResult
	err := c.Call(context.TODO(), "getrawtransaction", &resp, txHash, verbose)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// DecodeRawTransaction uses decoderawtransaction RPC to decode raw transaction bytes.
func (c *DcrdRPC) DecodeRawTransaction(txHex string) (*dcrdtypes.TxRawDecodeResult, error) {
	var resp dcrdtypes.TxRawDecodeResult
	err := c.Call(context.TODO(), "decoderawtransaction", &resp, txHex)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

// SendRawTransaction uses sendrawtransaction RPC to broadcast a transaction to
// the network. It ignores errors caused by duplicate transactions.
func (c *DcrdRPC) SendRawTransaction(txHex string) error {
	const allowHighFees = false
	err := c.Call(context.TODO(), "sendrawtransaction", nil, txHex, allowHighFees)
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

// NotifyBlocks uses notifyblocks RPC to request new block notifications from dcrd.
func (c *DcrdRPC) NotifyBlocks() error {
	return c.Call(context.TODO(), "notifyblocks", nil)
}

// GetBestBlockHeader uses getbestblockhash RPC, followed by getblockheader RPC,
// to retrieve the header of the best block known to the dcrd instance.
func (c *DcrdRPC) GetBestBlockHeader() (*wire.BlockHeader, error) {
	var bestBlockHash string
	err := c.Call(context.TODO(), "getbestblockhash", &bestBlockHash)
	if err != nil {
		return nil, err
	}

	blockHeader, err := c.GetBlockHeader(bestBlockHash)
	if err != nil {
		return nil, err
	}
	return blockHeader, nil
}

// GetBlockHeader uses getblockheader RPC with verbose=false to retrieve
// the header of the requested block.
func (c *DcrdRPC) GetBlockHeader(blockHash string) (*wire.BlockHeader, error) {
	const verbose = false
	var resp string
	err := c.Call(context.TODO(), "getblockheader", &resp, blockHash, verbose)
	if err != nil {
		return nil, err
	}

	// Decode the serialized block header hex to raw bytes.
	headerBytes, err := hex.DecodeString(resp)
	if err != nil {
		return nil, err
	}

	// Deserialize the block header and return it.
	var blockHeader wire.BlockHeader
	err = blockHeader.Deserialize(bytes.NewReader(headerBytes))
	if err != nil {
		return nil, err
	}

	return &blockHeader, nil
}

// ExistsLiveTicket uses existslivetickets RPC to check if the provided ticket
// hash is a live ticket known to the dcrd instance.
func (c *DcrdRPC) ExistsLiveTicket(ticketHash string) (bool, error) {
	var exists string
	err := c.Call(context.TODO(), "existslivetickets", &exists, []string{ticketHash})
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

func (c *DcrdRPC) GetBlock(hash string) (*wire.MsgBlock, error) {
	var resp string
	const verbose = false
	const verboseTx = false
	err := c.Call(context.TODO(), "getblock", &resp, hash, verbose, verboseTx)
	if err != nil {
		return nil, err
	}

	// Decode the serialized block hex to raw bytes.
	blockBytes, err := hex.DecodeString(resp)
	if err != nil {
		return nil, err
	}

	// Deserialize the block and return it.
	var msgBlock wire.MsgBlock
	err = msgBlock.Deserialize(bytes.NewReader(blockBytes))
	if err != nil {
		return nil, err
	}

	return &msgBlock, nil
}

func (c *DcrdRPC) GetBlockCount() (int64, error) {
	var count int64
	err := c.Call(context.TODO(), "getblockcount", &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (c *DcrdRPC) GetBlockHash(height int64) (string, error) {
	var resp string
	err := c.Call(context.TODO(), "getblockhash", &resp, height)
	if err != nil {
		return "", err
	}
	return resp, nil
}

// GetCFilterV2 retrieves the GCS filter for the provided block header,
// optionally verifies the inclusion proof, then returns the filter along with
// its key.
func (c *DcrdRPC) GetCFilterV2(header *wire.BlockHeader, verifyProof bool) ([gcs.KeySize]byte, *gcs.FilterV2, error) {
	var key [gcs.KeySize]byte
	var resp dcrdtypes.GetCFilterV2Result
	err := c.Call(context.TODO(), "getcfilterv2", &resp, header.BlockHash().String())
	if err != nil {
		return key, nil, fmt.Errorf("getcfilterv2 error: %w", err)
	}

	filterB, err := hex.DecodeString(resp.Data)
	if err != nil {
		return key, nil, fmt.Errorf("error decoding block filter: %w", err)
	}

	filter, err := gcs.FromBytesV2(blockcf2.B, blockcf2.M, filterB)
	if err != nil {
		return key, nil, fmt.Errorf("error decoding block filter: %w", err)
	}

	if verifyProof {
		filterHash := filter.Hash()

		proofHashes := make([]chainhash.Hash, len(resp.ProofHashes))
		for i, proofHash := range resp.ProofHashes {
			h, err := chainhash.NewHashFromStr(proofHash)
			if err != nil {
				return key, nil, err
			}
			proofHashes[i] = *h
		}

		if !standalone.VerifyInclusionProof(&header.StakeRoot, &filterHash, resp.ProofIndex, proofHashes) {
			return key, nil, fmt.Errorf("failed to verify inclusion proof: %w", err)
		}
	}

	return blockcf2.Key(&header.MerkleRoot), filter, nil
}
