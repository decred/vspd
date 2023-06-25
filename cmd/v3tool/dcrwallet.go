// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	wallettypes "decred.org/dcrwallet/v4/rpc/jsonrpc/types"
	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v4"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/txscript/v4/stdscript"
	"github.com/decred/dcrd/wire"
	"github.com/jrick/wsrpc/v2"
)

type dcrwallet struct {
	*wsrpc.Client
}

func newWalletRPC(rpcURL, rpcUser, rpcPass string) (*dcrwallet, error) {
	tlsOpt := wsrpc.WithTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
	})
	authOpt := wsrpc.WithBasicAuth(rpcUser, rpcPass)
	rpc, err := wsrpc.Dial(context.TODO(), rpcURL, tlsOpt, authOpt)
	if err != nil {
		return nil, err
	}
	return &dcrwallet{rpc}, nil
}

func (w *dcrwallet) createFeeTx(feeAddress string, fee int64) (string, error) {
	amounts := make(map[string]float64)
	amounts[feeAddress] = dcrutil.Amount(fee).ToCoin()

	var msgtxstr string
	err := w.Call(context.TODO(), "createrawtransaction", &msgtxstr, nil, amounts)
	if err != nil {
		return "", err
	}

	zero := int32(0)
	opt := wallettypes.FundRawTransactionOptions{
		ConfTarget: &zero,
	}
	var fundTx wallettypes.FundRawTransactionResult
	err = w.Call(context.TODO(), "fundrawtransaction", &fundTx, msgtxstr, "default", &opt)
	if err != nil {
		return "", err
	}

	tx := wire.NewMsgTx()
	err = tx.Deserialize(hex.NewDecoder(strings.NewReader(fundTx.Hex)))
	if err != nil {
		return "", err
	}

	transactions := make([]dcrdtypes.TransactionInput, 0)

	for _, v := range tx.TxIn {
		transactions = append(transactions, dcrdtypes.TransactionInput{
			Txid: v.PreviousOutPoint.Hash.String(),
			Vout: v.PreviousOutPoint.Index,
		})
	}

	var locked bool
	unlock := false
	err = w.Call(context.TODO(), "lockunspent", &locked, unlock, transactions)
	if err != nil {
		return "", err
	}

	if !locked {
		return "", errors.New("unspent output not locked")
	}

	var signedTx wallettypes.SignRawTransactionResult
	err = w.Call(context.TODO(), "signrawtransaction", &signedTx, fundTx.Hex)
	if err != nil {
		return "", err
	}
	if !signedTx.Complete {
		return "", fmt.Errorf("not all signed")
	}
	return signedTx.Hex, nil
}

func (w *dcrwallet) SignMessage(_ context.Context, msg string, commitmentAddr stdaddr.Address) ([]byte, error) {
	var signature string
	err := w.Call(context.TODO(), "signmessage", &signature, commitmentAddr.String(), msg)
	if err != nil {
		return nil, err
	}

	return base64.StdEncoding.DecodeString(signature)
}

func (w *dcrwallet) dumpPrivKey(addr stdaddr.Address) (string, error) {
	var privKeyStr string
	err := w.Call(context.TODO(), "dumpprivkey", &privKeyStr, addr.String())
	if err != nil {
		return "", err
	}
	return privKeyStr, nil
}

func (w *dcrwallet) getTickets() (*wallettypes.GetTicketsResult, error) {
	var tickets wallettypes.GetTicketsResult
	includeImmature := true
	err := w.Call(context.TODO(), "gettickets", &tickets, includeImmature)
	if err != nil {
		return nil, err
	}
	return &tickets, nil
}

// getTicketDetails returns the ticket hex, privkey for voting, and the
// commitment address.
func (w *dcrwallet) getTicketDetails(ticketHash string) (string, string, stdaddr.Address, error) {
	var getTransactionResult wallettypes.GetTransactionResult
	err := w.Call(context.TODO(), "gettransaction", &getTransactionResult, ticketHash, false)
	if err != nil {
		fmt.Printf("gettransaction: %v\n", err)
		return "", "", nil, err
	}

	msgTx := wire.NewMsgTx()
	if err = msgTx.Deserialize(hex.NewDecoder(strings.NewReader(getTransactionResult.Hex))); err != nil {
		return "", "", nil, err
	}
	if len(msgTx.TxOut) < 2 {
		return "", "", nil, errors.New("msgTx.TxOut < 2")
	}

	const scriptVersion = 0
	scriptType, submissionAddr := stdscript.ExtractAddrs(scriptVersion,
		msgTx.TxOut[0].PkScript, chaincfg.TestNet3Params())
	if scriptType == stdscript.STNonStandard {
		return "", "", nil, fmt.Errorf("invalid script version %d", scriptVersion)
	}
	if len(submissionAddr) != 1 {
		return "", "", nil, errors.New("submissionAddr != 1")
	}

	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript,
		chaincfg.TestNet3Params())
	if err != nil {
		return "", "", nil, err
	}

	privKeyStr, err := w.dumpPrivKey(submissionAddr[0])
	if err != nil {
		return "", "", nil, err
	}

	return getTransactionResult.Hex, privKeyStr, addr, nil
}
