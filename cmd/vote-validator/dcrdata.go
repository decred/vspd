// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

type txns struct {
	Transactions []string `json:"transactions"`
}

type txInputID struct {
	Hash  string `json:"hash"`
	Index uint32 `json:"vin_index"`
}

type vout struct {
	Value               float64                      `json:"value"`
	N                   uint32                       `json:"n"`
	Version             uint16                       `json:"version"`
	ScriptPubKeyDecoded dcrdtypes.ScriptPubKeyResult `json:"scriptPubKey"`
	Spend               *txInputID                   `json:"spend"`
}

type tx struct {
	TxID          string          `json:"txid"`
	Size          int32           `json:"size"`
	Version       int32           `json:"version"`
	Locktime      uint32          `json:"locktime"`
	Expiry        uint32          `json:"expiry"`
	Vin           []dcrdtypes.Vin `json:"vin"`
	Vout          []vout          `json:"vout"`
	Confirmations int64           `json:"confirmations"`
}

type dcrdataClient struct {
	URL string
}

func (d *dcrdataClient) txns(txnHashes []string, spends bool) ([]tx, error) {
	jsonData, err := json.Marshal(txns{
		Transactions: txnHashes,
	})
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/txs?spends=%t", d.URL, spends)
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dcrdata response: %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var txns []tx
	err = json.Unmarshal(body, &txns)
	if err != nil {
		return nil, err
	}

	return txns, nil
}
