package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
	"github.com/jrick/bitset"
	"github.com/jrick/wsrpc/v2"
)

const (
	requiredDcrdVersion = "6.1.1"
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
	*client
}

func SetupDcrd(user, pass, addr string, cert []byte, n wsrpc.Notifier) DcrdConnect {
	return DcrdConnect{setup(user, pass, addr, cert, n)}
}

// Client creates a new DcrdRPC client instance. Returns an error if dialing
// dcrd fails or if dcrd is misconfigured.
func (d *DcrdConnect) Client(ctx context.Context, netParams *chaincfg.Params) (*DcrdRPC, error) {
	c, newConnection, err := d.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("dcrd connection error: %v", err)
	}

	// If this is a reused connection, we don't need to validate the dcrd config
	// again.
	if !newConnection {
		return &DcrdRPC{c, ctx}, nil
	}

	// Verify dcrd is at the required api version.
	var verMap map[string]dcrdtypes.VersionResult
	err = c.Call(ctx, "version", &verMap)
	if err != nil {
		return nil, fmt.Errorf("dcrd version check failed: %v", err)
	}
	dcrdVersion, exists := verMap["dcrdjsonrpcapi"]
	if !exists {
		return nil, fmt.Errorf("dcrd version response missing 'dcrdjsonrpcapi'")
	}
	if dcrdVersion.VersionString != requiredDcrdVersion {
		return nil, fmt.Errorf("wrong dcrd RPC version: got %s, expected %s",
			dcrdVersion.VersionString, requiredDcrdVersion)
	}

	// Verify dcrd is on the correct network.
	var netID wire.CurrencyNet
	err = c.Call(ctx, "getcurrentnet", &netID)
	if err != nil {
		return nil, fmt.Errorf("dcrd getcurrentnet check failed: %v", err)
	}
	if netID != netParams.Net {
		return nil, fmt.Errorf("dcrd running on %s, expected %s", netID, netParams.Net)
	}

	// Verify dcrd has tx index enabled (required for getrawtransaction).
	var info dcrdtypes.InfoChainResult
	err = c.Call(ctx, "getinfo", &info)
	if err != nil {
		return nil, fmt.Errorf("dcrd getinfo check failed: %v", err)
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

func (c *DcrdRPC) SendRawTransaction(txHex string) error {
	allowHighFees := false
	err := c.Call(c.ctx, "sendrawtransaction", nil, txHex, allowHighFees)
	if err != nil {

		// sendrawtransaction can return two different errors when a tx already
		// exists:
		//
		// If the tx is in mempool...
		//  Error code: -40
		//  message: rejected transaction <hash>: already have transaction <hash>
		//
		// If the tx is in a mined block...
		//  Error code: -1
		//  message: rejected transaction <hash>: transaction already exists
		//
		// It's not a problem if the transaction has already been broadcast, so
		// we will capture these errors and return nil.

		// Exists in mempool.
		var e *wsrpc.Error
		if errors.As(err, &e) && e.Code == ErrRPCDuplicateTx {
			return nil
		}

		// Exists in mined block.
		// We cannot use error code -1 here because it is a generic code for
		// many errors, so we instead need to string match on the message.
		if strings.Contains(err.Error(), "transaction already exists") {
			return nil
		}

		return err
	}
	return nil
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
		return "", errors.New("invalid transaction - not sstx")
	}
	if len(msgTx.TxOut) != 3 {
		return "", fmt.Errorf("invalid transaction - expected 3 outputs, got %d", len(msgTx.TxOut))
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

// CanTicketVote checks determines whether a ticket is able to vote at some
// point in the future by checking that it is currently either immature or live.
func (c *DcrdRPC) CanTicketVote(rawTx *dcrdtypes.TxRawResult, ticketHash string, netParams *chaincfg.Params) (bool, error) {

	// Tickets which have more than (TicketMaturity+TicketExpiry+1)
	// confirmations are too old to vote.
	if rawTx.Confirmations > int64(uint32(netParams.TicketMaturity)+netParams.TicketExpiry)+1 {
		return false, nil
	}

	// If ticket is currently immature, it will be able to vote in future.
	if rawTx.Confirmations <= int64(netParams.TicketMaturity) {
		return true, nil
	}

	// If ticket is currently live, it will be able to vote in future.
	live, err := c.ExistsLiveTicket(ticketHash)
	if err != nil {
		return false, err
	}

	return live, nil
}
