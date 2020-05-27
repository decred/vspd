package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
)

const (
	requiredDcrdVersion = "6.1.1"
)

// DcrdRPC provides methods for calling dcrd JSON-RPCs without exposing the details
// of JSON encoding.
type DcrdRPC struct {
	Caller
	ctx context.Context
}

// DcrdClient creates a new DcrdRPC client instance from a caller.
func DcrdClient(ctx context.Context, c Caller, netParams *chaincfg.Params) (*DcrdRPC, error) {

	// Verify dcrd is at the required api version.
	var verMap map[string]dcrdtypes.VersionResult
	err := c.Call(ctx, "version", &verMap)
	if err != nil {
		return nil, fmt.Errorf("version check failed: %v", err)
	}
	dcrdVersion, exists := verMap["dcrdjsonrpcapi"]
	if !exists {
		return nil, fmt.Errorf("version response missing 'dcrdjsonrpcapi'")
	}
	if dcrdVersion.VersionString != requiredDcrdVersion {
		return nil, fmt.Errorf("wrong dcrd RPC version: got %s, expected %s",
			dcrdVersion.VersionString, requiredDcrdVersion)
	}

	// Verify dcrd is on the correct network.
	var netID wire.CurrencyNet
	err = c.Call(ctx, "getcurrentnet", &netID)
	if err != nil {
		return nil, fmt.Errorf("getcurrentnet check failed: %v", err)
	}
	if netID != netParams.Net {
		return nil, fmt.Errorf("dcrd running on %s, expected %s", netID, netParams.Net)
	}

	// Verify dcrd has tx index enabled (required for getrawtransaction).
	var info dcrdtypes.InfoChainResult
	err = c.Call(ctx, "getinfo", &info)
	if err != nil {
		return nil, fmt.Errorf("getinfo check failed: %v", err)
	}
	if !info.TxIndex {
		return nil, errors.New("dcrd does not have transaction index enabled (--txindex)")
	}

	return &DcrdRPC{c, ctx}, nil
}

func (c *DcrdRPC) GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error) {
	verbose := 1
	var resp dcrdtypes.TxRawResult
	err := c.Call(c.ctx, "getrawtransaction", &resp, txHash, verbose)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *DcrdRPC) SendRawTransaction(txHex string) (string, error) {
	allowHighFees := false
	var txHash string
	err := c.Call(c.ctx, "sendrawtransaction", &txHash, txHex, allowHighFees)
	if err != nil {
		return "", err
	}
	return txHash, nil
}

func (c *DcrdRPC) GetTicketCommitmentAddress(ticketHash string, netParams *chaincfg.Params) (string, error) {
	// Retrieve and parse the transaction.
	resp, err := c.GetRawTransaction(ticketHash)
	if err != nil {
		return "", err
	}
	msgHex, err := hex.DecodeString(resp.Hex)
	if err != nil {
		return "", err
	}
	msgTx := wire.NewMsgTx()
	if err = msgTx.FromBytes(msgHex); err != nil {
		return "", err
	}

	// Ensure transaction is a valid ticket.
	if !stake.IsSStx(msgTx) {
		return "", errors.New("invalid transcation - not sstx")
	}
	if len(msgTx.TxOut) != 3 {
		return "", fmt.Errorf("invalid transcation - expected 3 outputs, got %d", len(msgTx.TxOut))
	}

	// Get ticket commitment address.
	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, netParams)
	if err != nil {
		return "", err
	}

	return addr.Address(), nil
}

func (c *DcrdRPC) NotifyBlocks() error {
	return c.Call(c.ctx, "notifyblocks", nil)
}
