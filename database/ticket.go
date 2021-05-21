// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// FeeStatus represents the current state of a ticket fee payment.
type FeeStatus string

const (
	// NoFee indicates no fee tx has been received yet.
	NoFee FeeStatus = "none"
	// FeeReceieved indicates fee tx has been received but not broadcast.
	FeeReceieved FeeStatus = "received"
	// FeeBroadcast indicates fee tx has been broadcast but not confirmed.
	FeeBroadcast FeeStatus = "broadcast"
	// FeeConfirmed indicates fee tx has been broadcast and confirmed.
	FeeConfirmed FeeStatus = "confirmed"
	// FeeError indicates fee tx could not be broadcast due to an error.
	FeeError FeeStatus = "error"
)

// TicketOutcome describes the reason a ticket is no longer votable.
type TicketOutcome string

const (
	// Revoked indicates the ticket has been revoked, either because it was
	// missed or it expired.
	Revoked TicketOutcome = "revoked"
	// Voted indicates the ticket has already voted.
	Voted TicketOutcome = "voted"
)

// Ticket is serialized to json and stored in bbolt db. The json keys are
// deliberately kept short because they are duplicated many times in the db.
type Ticket struct {
	Hash              string `json:"hsh"`
	PurchaseHeight    int64  `json:"phgt"`
	CommitmentAddress string `json:"cmtaddr"`
	FeeAddressIndex   uint32 `json:"faddridx"`
	FeeAddress        string `json:"faddr"`
	FeeAmount         int64  `json:"famt"`
	FeeExpiration     int64  `json:"fexp"`

	// Confirmed will be set when the ticket has 6+ confirmations.
	Confirmed bool `json:"conf"`

	// VotingWIF is set in /payfee.
	VotingWIF string `json:"vwif"`

	// VoteChoices is initially set in /payfee, but can be updated in
	// /setvotechoices.
	VoteChoices map[string]string `json:"vchces"`

	// FeeTxHex and FeeTxHash will be set when the fee tx has been received.
	FeeTxHex  string `json:"fhex"`
	FeeTxHash string `json:"fhsh"`

	// FeeTxStatus indicates the current state of the fee transaction.
	FeeTxStatus FeeStatus `json:"fsts"`

	// Outcome is set once a ticket is either voted or revoked. An empty outcome
	// indicates that a ticket is still votable.
	Outcome TicketOutcome `json:"otcme"`
}

func (t *Ticket) FeeExpired() bool {
	now := time.Now()
	return now.After(time.Unix(t.FeeExpiration, 0))
}

// InsertNewTicket will insert the provided ticket into the database. Returns an
// error if either the ticket hash or fee address already exist.
func (vdb *VspDatabase) InsertNewTicket(ticket Ticket) error {
	vdb.ticketsMtx.Lock()
	defer vdb.ticketsMtx.Unlock()

	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		hashBytes := []byte(ticket.Hash)

		if ticketBkt.Get(hashBytes) != nil {
			return fmt.Errorf("ticket already exists with hash %s", ticket.Hash)
		}

		// Error if a ticket already exists with the same fee address.
		err := ticketBkt.ForEach(func(k, v []byte) error {
			var t Ticket
			err := json.Unmarshal(v, &t)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %w", err)
			}

			if t.FeeAddress == ticket.FeeAddress {
				return fmt.Errorf("ticket with fee address %s already exists", t.FeeAddress)
			}

			return nil
		})
		if err != nil {
			return err
		}

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %w", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) DeleteTicket(ticket Ticket) error {
	vdb.ticketsMtx.Lock()
	defer vdb.ticketsMtx.Unlock()

	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		err := ticketBkt.Delete([]byte(ticket.Hash))
		if err != nil {
			return fmt.Errorf("could not delete ticket: %w", err)
		}

		return nil
	})
}

func (vdb *VspDatabase) UpdateTicket(ticket Ticket) error {
	vdb.ticketsMtx.Lock()
	defer vdb.ticketsMtx.Unlock()

	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		hashBytes := []byte(ticket.Hash)

		if ticketBkt.Get(hashBytes) == nil {
			return fmt.Errorf("ticket does not exist with hash %s", ticket.Hash)
		}

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %w", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) GetTicketByHash(ticketHash string) (Ticket, bool, error) {
	vdb.ticketsMtx.RLock()
	defer vdb.ticketsMtx.RUnlock()

	var ticket Ticket
	var found bool
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		ticketBytes := ticketBkt.Get([]byte(ticketHash))
		if ticketBytes == nil {
			return nil
		}

		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %w", err)
		}

		found = true

		return nil
	})

	return ticket, found, err
}

// CountTickets returns the total number of voted, revoked, and currently voting
// tickets. Requires deserializing every ticket in the db so should be used
// sparingly.
func (vdb *VspDatabase) CountTickets() (int64, int64, int64, error) {
	vdb.ticketsMtx.RLock()
	defer vdb.ticketsMtx.RUnlock()

	var voting, voted, revoked int64
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %w", err)
			}

			if ticket.FeeTxStatus == FeeConfirmed {
				switch ticket.Outcome {
				case Voted:
					voted++
				case Revoked:
					revoked++
				default:
					voting++
				}
			}

			return nil
		})
	})

	return voting, voted, revoked, err
}

// GetUnconfirmedTickets returns tickets which are not yet confirmed.
func (vdb *VspDatabase) GetUnconfirmedTickets() ([]Ticket, error) {
	vdb.ticketsMtx.RLock()
	defer vdb.ticketsMtx.RUnlock()

	return vdb.filterTickets(func(t Ticket) bool {
		return !t.Confirmed
	})
}

// GetPendingFees returns tickets which are confirmed and have a fee tx which is
// not yet broadcast.
func (vdb *VspDatabase) GetPendingFees() ([]Ticket, error) {
	vdb.ticketsMtx.RLock()
	defer vdb.ticketsMtx.RUnlock()

	return vdb.filterTickets(func(t Ticket) bool {
		return t.Confirmed && t.FeeTxStatus == FeeReceieved
	})
}

// GetUnconfirmedFees returns tickets with a fee tx that is broadcast but not
// confirmed yet.
func (vdb *VspDatabase) GetUnconfirmedFees() ([]Ticket, error) {
	vdb.ticketsMtx.RLock()
	defer vdb.ticketsMtx.RUnlock()

	return vdb.filterTickets(func(t Ticket) bool {
		return t.FeeTxStatus == FeeBroadcast
	})
}

// GetVotableTickets returns tickets with a confirmed fee tx and no outcome (ie.
// not expired/voted/missed).
func (vdb *VspDatabase) GetVotableTickets() ([]Ticket, error) {
	return vdb.filterTickets(func(t Ticket) bool {
		return t.FeeTxStatus == FeeConfirmed && t.Outcome == ""
	})
}

// filterTickets accepts a filter function and returns all tickets from the
// database which match the filter.
//
// This function must be called with the lock held.
func (vdb *VspDatabase) filterTickets(filter func(Ticket) bool) ([]Ticket, error) {
	var tickets []Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %w", err)
			}

			if filter(ticket) {
				tickets = append(tickets, ticket)
			}

			return nil
		})
	})

	return tickets, err
}
