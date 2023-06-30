// Copyright (c) 2022-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package client

import (
	"context"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

type DatabaseTicket interface {
	Host2() string
	FeeTxConfirmed() bool
	FeeTxPaid() bool
	FeeTxStarted() bool
	FeeTxErrored() bool
}

type Wallet interface {
	Spender(ctx context.Context, out *wire.OutPoint) (*wire.MsgTx, uint32, error)
	MainChainTip(ctx context.Context) (hash chainhash.Hash, height int32)
	TxBlock(ctx context.Context, hash *chainhash.Hash) (chainhash.Hash, int32, error)
	DumpWIFPrivateKey(ctx context.Context, addr stdaddr.Address) (string, error)
	VSPFeeHashForTicket(ctx context.Context, ticketHash *chainhash.Hash) (chainhash.Hash, error)
	UpdateVspTicketFeeToStarted(ctx context.Context, ticketHash, feeHash *chainhash.Hash, host string, pubkey []byte) error
	GetTransactionsByHashes(ctx context.Context, txHashes []*chainhash.Hash) (txs []*wire.MsgTx, notFound []*wire.InvVect, err error)
	SetPublished(ctx context.Context, hash *chainhash.Hash, published bool) error
	UpdateVspTicketFeeToPaid(ctx context.Context, ticketHash, feeHash *chainhash.Hash, host string, pubkey []byte) error
	UpdateVspTicketFeeToErrored(ctx context.Context, ticketHash *chainhash.Hash, host string, pubkey []byte) error
	AgendaChoices(ctx context.Context, ticketHash *chainhash.Hash) (choices map[string]string, voteBits uint16, err error)
	TSpendPolicyForTicket(ticketHash *chainhash.Hash) map[string]string
	TreasuryKeyPolicyForTicket(ticketHash *chainhash.Hash) map[string]string
	AbandonTransaction(ctx context.Context, hash *chainhash.Hash) error
	TxConfirms(ctx context.Context, hash *chainhash.Hash) (int32, error)
	ForUnspentUnexpiredTickets(ctx context.Context, f func(hash *chainhash.Hash) error) error
	IsVSPTicketConfirmed(ctx context.Context, ticketHash *chainhash.Hash) (bool, error)
	UpdateVspTicketFeeToConfirmed(ctx context.Context, ticketHash, feeHash *chainhash.Hash, host string, pubkey []byte) error
	VSPTicketInfo(ctx context.Context, ticketHash *chainhash.Hash) (DatabaseTicket, error)
	SignMessage(ctx context.Context, msg string, addr stdaddr.Address) (sig []byte, err error)
	CreateVspPayment(ctx context.Context, tx *wire.MsgTx, fee dcrutil.Amount, feeAddr stdaddr.Address, feeAcct uint32, changeAcct uint32) error
}
