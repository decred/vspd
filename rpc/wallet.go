package rpc

import (
	"context"

	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
)

// WalletRPC provides methods for calling dcrwallet JSON-RPCs without exposing the details
// of JSON encoding.
type WalletRPC struct {
	Caller
}

// WalletClient creates a new WalletRPC client instance from a caller.
func WalletClient(caller Caller) *WalletRPC {
	return &WalletRPC{caller}
}

func (c *WalletRPC) ImportXPub(ctx context.Context, account, xpub string) error {
	return c.Call(ctx, "importxpub", nil, account, xpub)
}

func (c *WalletRPC) GetMasterPubKey(ctx context.Context, account string) (string, error) {
	var pubKey string
	err := c.Call(ctx, "getmasterpubkey", &pubKey, account)
	if err != nil {
		return "", err
	}
	return pubKey, nil
}

func (c *WalletRPC) ListAccounts(ctx context.Context) (map[string]float64, error) {
	var accounts map[string]float64
	err := c.Call(ctx, "listaccounts", &accounts)
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

func (c *WalletRPC) GetNewAddress(ctx context.Context, account string) (string, error) {
	var newAddress string
	err := c.Call(ctx, "getnewaddress", &newAddress, account)
	if err != nil {
		return "", err
	}
	return newAddress, nil
}

func (c *WalletRPC) GetBlockHeader(ctx context.Context, blockHash string) (*dcrdtypes.GetBlockHeaderVerboseResult, error) {
	verbose := true
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err := c.Call(ctx, "getblockheader", &blockHeader, blockHash, verbose)
	if err != nil {
		return nil, err
	}
	return &blockHeader, nil
}

func (c *WalletRPC) GetRawTransaction(ctx context.Context, txHash string) (*dcrdtypes.TxRawResult, error) {
	verbose := 1
	var resp dcrdtypes.TxRawResult
	err := c.Call(ctx, "getrawtransaction", &resp, txHash, verbose)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *WalletRPC) SendRawTransaction(ctx context.Context, txHex string) (string, error) {
	allowHighFees := false
	var txHash string
	err := c.Call(ctx, "sendrawtransaction", &txHash, txHex, allowHighFees)
	if err != nil {
		return "", err
	}
	return txHash, nil
}

func (c *WalletRPC) AddTransaction(ctx context.Context, blockHash, txHex string) error {
	return c.Call(ctx, "addtransaction", nil, blockHash, txHex)
}

func (c *WalletRPC) ImportPrivKey(ctx context.Context, votingWIF string) error {
	label := "imported"
	rescan := false
	scanFrom := 0
	return c.Call(ctx, "importprivkey", nil, votingWIF, label, rescan, scanFrom)
}

func (c *WalletRPC) SetVoteChoice(ctx context.Context, agenda, choice, ticketHash string) error {
	return c.Call(ctx, "setvotechoice", nil, agenda, choice, ticketHash)
}
