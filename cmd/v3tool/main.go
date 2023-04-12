// Copyright (c) 2022-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/decred/slog"
	"github.com/decred/vspd/client/v2"
	"github.com/decred/vspd/types/v2"
)

const (
	vspdURL = "http://localhost:8800"
	// dcrwallet RPC.
	rpcURL  = "wss://localhost:19110/ws"
	rpcUser = "user"
	rpcPass = "pass"
)

func getVspPubKey(url string) ([]byte, error) {
	resp, err := http.Get(url + "/api/v3/vspinfo")
	if err != nil {
		return nil, err
	}

	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	var j types.VspInfoResponse
	err = json.Unmarshal(b, &j)
	if err != nil {
		return nil, err
	}

	err = client.ValidateServerSignature(resp, b, j.PubKey)
	if err != nil {
		return nil, err
	}

	return j.PubKey, nil
}

func run() int {
	log := slog.NewBackend(os.Stdout).Logger("")
	log.SetLevel(slog.LevelTrace)

	walletRPC, err := newWalletRPC(rpcURL, rpcUser, rpcPass)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}
	defer walletRPC.Close()

	log.Infof("vpsd url: %s", vspdURL)

	pubKey, err := getVspPubKey(vspdURL)
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	log.Infof("vspd pubkey: %x", pubKey)

	vClient := client.Client{
		URL:    vspdURL,
		PubKey: pubKey,
		Sign:   walletRPC.SignMessage,
		Log:    log,
	}

	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	// Get list of tickets
	tickets, err := walletRPC.getTickets()
	if err != nil {
		log.Errorf("%v", err)
		return 1
	}

	if len(tickets.Hashes) == 0 {
		log.Errorf("wallet owns no tickets")
		return 1
	}

	log.Infof("wallet returned %d ticket(s):", len(tickets.Hashes))
	for _, tkt := range tickets.Hashes {
		log.Infof("    %s", tkt)
	}

	for i := 0; i < len(tickets.Hashes); i++ {
		ticketHash := tickets.Hashes[i]
		hex, privKeyStr, commitmentAddr, err := walletRPC.getTicketDetails(ticketHash)
		if err != nil {
			log.Errorf("%v", err)
			return 1
		}

		log.Infof("")
		log.Infof("Processing ticket %d of %d:", i+1, len(tickets.Hashes))
		log.Infof("    Hash: %s", ticketHash)
		log.Infof("    privKeyStr: %s", privKeyStr)
		log.Infof("    commitmentAddr: %s", commitmentAddr)
		log.Infof("")

		feeAddrReq := types.FeeAddressRequest{
			TicketHex: hex,
			// Hack for ParentHex, can't be bothered to get the real one. It doesn't
			// make a difference when testing locally anyway.
			ParentHex:  hex,
			TicketHash: ticketHash,
			Timestamp:  time.Now().Unix(),
		}

		feeAddrResp, err := vClient.FeeAddress(context.TODO(), feeAddrReq, commitmentAddr)
		if err != nil {
			log.Errorf("getFeeAddress error: %v", err)
			break
		}

		log.Infof("feeAddress: %v", feeAddrResp.FeeAddress)
		log.Infof("privKeyStr: %v", privKeyStr)

		feeTx, err := walletRPC.createFeeTx(feeAddrResp.FeeAddress, feeAddrResp.FeeAmount)
		if err != nil {
			log.Errorf("createFeeTx error: %v", err)
			break
		}

		voteChoices := map[string]string{"autorevocations": "no"}
		tspend := map[string]string{
			"6c78690fa2fa31803df0376897725704e9dc19ecbdf80061e79b69de93ca1360": "no",
			"abb86660dda1f1b66544bab24a823a22e9213ada48649f0d913623f49e17dacb": "yes",
		}
		treasury := map[string]string{
			"03f6e7041f1cf51ee10e0a01cd2b0385ce3cd9debaabb2296f7e9dee9329da946c": "no",
			"0319a37405cb4d1691971847d7719cfce70857c0f6e97d7c9174a3998cf0ab86dd": "yes",
		}

		payFeeReq := types.PayFeeRequest{
			FeeTx:          feeTx,
			VotingKey:      privKeyStr,
			TicketHash:     ticketHash,
			Timestamp:      time.Now().Unix(),
			VoteChoices:    voteChoices,
			TSpendPolicy:   tspend,
			TreasuryPolicy: treasury,
		}

		_, err = vClient.PayFee(context.TODO(), payFeeReq, commitmentAddr)
		if err != nil {
			log.Errorf("payFee error: %v", err)
			continue
		}

		ticketStatusReq := types.TicketStatusRequest{
			TicketHash: ticketHash,
		}

		_, err = vClient.TicketStatus(context.TODO(), ticketStatusReq, commitmentAddr)
		if err != nil {
			log.Errorf("getTicketStatus error: %v", err)
			break
		}

		voteChoices["autorevocations"] = "yes"

		// Sleep to ensure a new timestamp. vspd will reject old/reused timestamps.
		time.Sleep(1001 * time.Millisecond)

		voteChoiceReq := types.SetVoteChoicesRequest{
			Timestamp:      time.Now().Unix(),
			TicketHash:     ticketHash,
			VoteChoices:    voteChoices,
			TSpendPolicy:   tspend,
			TreasuryPolicy: treasury,
		}

		_, err = vClient.SetVoteChoices(context.TODO(), voteChoiceReq, commitmentAddr)
		if err != nil {
			log.Errorf("setVoteChoices error: %v", err)
			break
		}

		_, err = vClient.TicketStatus(context.TODO(), ticketStatusReq, commitmentAddr)
		if err != nil {
			log.Errorf("getTicketStatus error: %v", err)
			break
		}

		time.Sleep(1 * time.Second)
	}

	return 0
}

func main() {
	os.Exit(run())
}
