package rpc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

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

// DcrdRPC provides methods for calling dcrd JSON-RPCs without exposing the details
// of JSON encoding.
type DcrdRPC struct {
	Caller
	ctx context.Context
}

type DcrdConnect connect

func SetupDcrd(ctx context.Context, shutdownWg *sync.WaitGroup, user, pass, addr string, cert []byte, n wsrpc.Notifier) DcrdConnect {
	return DcrdConnect(setup(ctx, shutdownWg, user, pass, addr, cert, n))
}

// Client creates a new DcrdRPC client instance. Returns an error if dialing
// dcrd fails or if dcrd is misconfigured.
func (d *DcrdConnect) Client(ctx context.Context, netParams *chaincfg.Params) (*DcrdRPC, error) {

	c, newConnection, err := connect(*d)()
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

func (c *DcrdRPC) SendRawTransaction(txHex string) (string, error) {
	allowHighFees := false
	var txHash string
	err := c.Call(c.ctx, "sendrawtransaction", &txHash, txHex, allowHighFees)
	if err != nil {
		// It's not a problem if the transaction has already been broadcast,
		// just need to calculate and return its hash.
		if !strings.Contains(err.Error(), "transaction already exists") {
			return "", err
		}

		msgHex, err := hex.DecodeString(txHex)
		if err != nil {
			return "", fmt.Errorf("DecodeString error: %v", err)

		}
		msgTx := wire.NewMsgTx()
		if err = msgTx.FromBytes(msgHex); err != nil {
			return "", fmt.Errorf("FromBytes error: %v", err)

		}

		txHash = msgTx.TxHash().String()
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
