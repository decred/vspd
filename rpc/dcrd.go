package rpc

import (
	"context"
	"encoding/hex"
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
func DcrdClient(ctx context.Context, c Caller) (*DcrdRPC, error) {

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

	// TODO: Ensure correct network.

	return &DcrdRPC{c, ctx}, nil
}

func (c *DcrdRPC) GetBlockHeader(blockHash string) (*dcrdtypes.GetBlockHeaderVerboseResult, error) {
	verbose := true
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err := c.Call(c.ctx, "getblockheader", &blockHeader, blockHash, verbose)
	if err != nil {
		return nil, err
	}
	return &blockHeader, nil
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
	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, netParams)
	if err != nil {
		return "", err
	}

	return addr.Address(), nil
}
