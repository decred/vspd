package main

import "github.com/decred/dcrd/dcrutil/v3"

type WalletClient struct {
}

func (db *WalletClient) AddTicket(tx *dcrutil.Tx) error {
	return  nil
}

func (db *WalletClient) ImportPrivKeyRescanFrom(w *dcrutil.WIF, s string, b bool, i int) error {
	return nil
}
