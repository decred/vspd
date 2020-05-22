package rpc

import (
	"context"
	"fmt"

	wallettypes "decred.org/dcrwallet/rpc/jsonrpc/types"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
)

const (
	requiredVotingWalletVersion = "8.1.0"
)

// VotingWalletRPC provides methods for calling dcrwallet JSON-RPCs without exposing the details
// of JSON encoding.
type VotingWalletRPC struct {
	Caller
	ctx context.Context
}

// VotingWalletClient creates a new VotingWalletRPC client instance from a caller.
func VotingWalletClient(ctx context.Context, c Caller) (*VotingWalletRPC, error) {

	// Verify dcrwallet is at the required api version.
	var verMap map[string]dcrdtypes.VersionResult
	err := c.Call(ctx, "version", &verMap)
	if err != nil {
		return nil, fmt.Errorf("version check failed: %v", err)
	}
	walletVersion, exists := verMap["dcrwalletjsonrpcapi"]
	if !exists {
		return nil, fmt.Errorf("version response missing 'dcrwalletjsonrpcapi'")
	}
	if walletVersion.VersionString != requiredVotingWalletVersion {
		return nil, fmt.Errorf("wrong dcrwallet RPC version: expected %s, got %s",
			walletVersion.VersionString, requiredVotingWalletVersion)
	}

	// Verify dcrwallet is voting, unlocked, and is connected to dcrd (not SPV).
	var walletInfo wallettypes.WalletInfoResult
	err = c.Call(ctx, "walletinfo", &walletInfo)
	if err != nil {
		return nil, fmt.Errorf("walletinfo check failed: %v", err)
	}
	if !walletInfo.Voting {
		return nil, fmt.Errorf("wallet has voting disabled")
	}
	if !walletInfo.Unlocked {
		return nil, fmt.Errorf("wallet is not unlocked")
	}
	if !walletInfo.DaemonConnected {
		return nil, fmt.Errorf("wallet is not connected to dcrd")
	}

	// TODO: Ensure correct network.

	return &VotingWalletRPC{c, ctx}, nil
}

func (c *VotingWalletRPC) AddTransaction(blockHash, txHex string) error {
	return c.Call(c.ctx, "addtransaction", nil, blockHash, txHex)
}

func (c *VotingWalletRPC) ImportPrivKey(votingWIF string) error {
	label := "imported"
	rescan := false
	scanFrom := 0
	return c.Call(c.ctx, "importprivkey", nil, votingWIF, label, rescan, scanFrom)
}

func (c *VotingWalletRPC) SetVoteChoice(agenda, choice, ticketHash string) error {
	return c.Call(c.ctx, "setvotechoice", nil, agenda, choice, ticketHash)
}
