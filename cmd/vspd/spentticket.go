// Copyright (c) 2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.package main

package main

import (
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
	"github.com/decred/vspd/database"
)

type spentTicket struct {
	dbTicket     database.Ticket
	expiryHeight int64
	heightSpent  int64
	spendingTx   *wire.MsgTx
}

func (s *spentTicket) voted() bool {
	return stake.IsSSGen(s.spendingTx)
}

func (s *spentTicket) missed() bool {
	// The following switch statement is a heuristic to estimate whether a
	// ticket was missed or expired based on its revoke height. Absolute
	// precision is not needed here as this status is only used to report VSP
	// stats via /vspinfo, which could be forged by a malicious VSP operator
	// anyway.
	switch {
	case s.heightSpent < s.expiryHeight:
		// A ticket revoked before expiry height was definitely missed.
		return true
	case s.heightSpent == s.expiryHeight:
		// If a ticket was revoked on exactly expiry height, assume it expired.
		// This might be incorrect if DCP-0009 was not active and a missed
		// ticket was coincidentally revoked on exactly the expiry height.
		return false
	case s.heightSpent == s.expiryHeight+1:
		// Revoking after the expiry height was only possible before DCP-0009
		// activated. Cannot be certain if missed or expired, but if it was
		// revoked exactly in the first block an expired ticket could have
		// possibly been revoked, there is a high probability the voter was
		// online and didn't miss the vote, so assume expired.
		return false
	default:
		// Revoking after the expiry height was only possible before DCP-0009
		// activated. Cannot be certain if missed or expired, but if it was
		// revoked later than the first block an expired ticket could have
		// possibly been revoked, it is probably because the voter was offline
		// and there is a much higher probability that the ticket was missed, so
		// assume missed.
		return true
	}
}

// findSpentTickets attempts to find transactions that vote/revoke the provided
// tickets by matching the payment script of the ticket's commitment address
// against the block filters of the mainchain blocks between the provided start
// block and the current best block. Returns any found spent tickets and the
// height of the most recent scanned block.
func (v *vspd) findSpentTickets(toCheck database.TicketList, startHeight int64) ([]spentTicket, int64, error) {
	params := v.cfg.netParams

	dcrdClient, _, err := v.dcrd.Client()
	if err != nil {
		return nil, 0, err
	}

	endHeight, err := dcrdClient.GetBlockCount()
	if err != nil {
		return nil, 0, fmt.Errorf("dcrd.GetBlockCount error: %w", err)
	}

	if startHeight > endHeight {
		return nil, 0, fmt.Errorf("start height %d greater than best block height %d",
			startHeight, endHeight)
	}

	numBlocks := 1 + endHeight - startHeight

	// Only log if checking a larger number of blocks to avoid spam.
	if numBlocks > 5 {
		v.log.Debugf("Scanning %d blocks for %s",
			numBlocks, pluralize(len(toCheck), "spent ticket"))
	}

	// Get commitment address payment scripts and parse hashes for each ticket
	// prior to the main loop. Two slices are needed because payment scripts
	// must be in their own slice to be passed into the MatchAny func.
	type ticketTuple struct {
		dbTicket database.Ticket
		hash     chainhash.Hash
	}

	tickets := make([]ticketTuple, 0, len(toCheck))
	scripts := make([][]byte, 0, len(toCheck))
	for _, ticket := range toCheck {
		parsedAddr, err := stdaddr.DecodeAddress(ticket.CommitmentAddress, params)
		if err != nil {
			return nil, 0, err
		}
		_, script := parsedAddr.PaymentScript()

		hash, err := chainhash.NewHashFromStr(ticket.Hash)
		if err != nil {
			return nil, 0, err
		}

		tickets = append(tickets, ticketTuple{ticket, *hash})
		scripts = append(scripts, script)
	}

	spent := make([]spentTicket, 0)

	for iHeight := startHeight; iHeight <= endHeight; iHeight++ {
		iHash, err := dcrdClient.GetBlockHash(iHeight)
		if err != nil {
			return nil, 0, err
		}

		iHeader, err := dcrdClient.GetBlockHeader(iHash)
		if err != nil {
			return nil, 0, err
		}

		verifyProof := v.cfg.netParams.dcp5Active(iHeight)
		key, filter, err := dcrdClient.GetCFilterV2(iHeader, verifyProof)
		if err != nil {
			return nil, 0, err
		}

		if !filter.MatchAny(key, scripts) {
			// No tickets are spent in this block, continue to the next one.
			continue
		}

		// Filter match means a ticket is likely spent in this block. Get the
		// full block to confirm.
		iBlock, err := dcrdClient.GetBlock(iHash)
		if err != nil {
			return nil, 0, err
		}

		for i := 0; i < len(tickets); i++ {
			// The regular transaction tree does not need to be checked because
			// tickets can only be spent by vote or revoke transactions which
			// are always in the stake tree.
			for _, blkTx := range iBlock.STransactions {
				if !txSpendsTicket(blkTx, tickets[i].hash) {
					continue
				}

				// Confirmed - ticket is spent in block.

				spent = append(spent, spentTicket{
					dbTicket:     tickets[i].dbTicket,
					expiryHeight: tickets[i].dbTicket.PurchaseHeight + int64(params.TicketMaturity) + int64(params.TicketExpiry),
					heightSpent:  iHeight,
					spendingTx:   blkTx,
				})

				// Remove this ticket and its script before continuing with the
				// next one.
				tickets = nonOrderPreservingRemove(tickets, i)
				scripts = nonOrderPreservingRemove(scripts, i)

				// Current index has been removed which means everything else
				// moved up one and thus the same index needs to be repeated.
				i--
			}
		}

		if len(tickets) == 0 {
			// Found spenders for all tickets, stop searching.
			break
		}
	}

	return spent, endHeight, nil
}

// txSpendsTicket returns true if the passed tx has an input that spends the
// specified output.
func txSpendsTicket(tx *wire.MsgTx, outputHash chainhash.Hash) bool {
	for _, txIn := range tx.TxIn {
		prevOut := &txIn.PreviousOutPoint
		if prevOut.Index == 0 && prevOut.Hash == outputHash {
			return true // Found spender.
		}
	}
	return false
}

// nonOrderPreservingRemove removes index i from slice s. The order of the slice
// is not preserved, however the important property of the function is that
// removing the same index from two slices of identical length will modify the
// order of each slice in the same way. This allows a 1-to-1 mapping between two
// (or more) slices to be preserved.
func nonOrderPreservingRemove[T any](s []T, i int) []T {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

// pluralize suffixes the provided noun with "s" if n is not 1, then
// concatenates n and noun with a space between them. For example:
//
//	(0, "biscuit") will return "0 biscuits"
//	(1, "biscuit") will return "1 biscuit"
//	(3, "biscuit") will return "3 biscuits"
func pluralize(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return fmt.Sprintf("%d %s", n, noun)
}
