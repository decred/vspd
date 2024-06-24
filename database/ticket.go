// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"fmt"
	"sort"
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
	// Expired indicates the ticket expired and has been revoked.
	Expired TicketOutcome = "expired"
	// Missed indicates the ticket was missed and has been revoked.
	Missed TicketOutcome = "missed"
	// Voted indicates the ticket has already voted.
	Voted TicketOutcome = "voted"

	// Revoked is a deprecated status which should no longer be used. It was
	// used before vspd was able to distinguish between expired and missed
	// tickets.
	Revoked TicketOutcome = "revoked"
)

// The keys used to store ticket values in the database.
var (
	hashK              = []byte("Hash")
	purchaseHeightK    = []byte("PurchaseHeight")
	commitmentAddressK = []byte("CommitmentAddress")
	feeAddressXPubIDK  = []byte("FeeAddressXPubID")
	feeAddressIndexK   = []byte("FeeAddressIndex")
	feeAddressK        = []byte("FeeAddress")
	feeAmountK         = []byte("FeeAmount")
	feeExpirationK     = []byte("FeeExpiration")
	confirmedK         = []byte("Confirmed")
	votingWIFK         = []byte("VotingWIF")
	voteChoicesK       = []byte("VoteChoices")
	tSpendPolicyK      = []byte("TSpendPolicy")
	treasuryPolicyK    = []byte("TreasuryPolicy")
	feeTxHexK          = []byte("FeeTxHex")
	feeTxHashK         = []byte("FeeTxHash")
	feeTxStatusK       = []byte("FeeTxStatus")
	outcomeK           = []byte("Outcome")
)

type Ticket struct {
	Hash              string
	PurchaseHeight    int64
	CommitmentAddress string
	FeeAddressXPubID  uint32
	FeeAddressIndex   uint32
	FeeAddress        string
	FeeAmount         int64
	FeeExpiration     int64

	// Confirmed will be set when the ticket has 6+ confirmations.
	Confirmed bool

	// VotingWIF is set in /payfee.
	VotingWIF string

	// VoteChoices, TSpendPolicy and TreasuryPolicy are initially set in
	// /payfee, but can be updated in /setvotechoices.
	VoteChoices    map[string]string
	TSpendPolicy   map[string]string
	TreasuryPolicy map[string]string

	// FeeTxHex and FeeTxHash will be set when the fee tx has been received.
	FeeTxHex  string
	FeeTxHash string

	// FeeTxStatus indicates the current state of the fee transaction.
	FeeTxStatus FeeStatus

	// Outcome is set once a ticket is either voted or revoked. An empty outcome
	// indicates that a ticket is still votable.
	Outcome TicketOutcome
}

type TicketList []Ticket

// EarliestPurchaseHeight returns the lowest non-zero purchase height in the
// list of tickets. Zero will be returned if the list is empty, or if every
// ticket in the list has zero purchase height.
func (t TicketList) EarliestPurchaseHeight() int64 {
	var oldestHeight int64
	for _, ticket := range t {
		// Skip unconfirmed tickets.
		if ticket.PurchaseHeight == 0 {
			continue
		}
		if oldestHeight == 0 || oldestHeight > ticket.PurchaseHeight {
			oldestHeight = ticket.PurchaseHeight
		}
	}
	return oldestHeight
}

func (t TicketList) SortByPurchaseHeight() {
	sort.Slice(t, func(i, j int) bool {
		return t[i].PurchaseHeight > t[j].PurchaseHeight
	})
}

func (t *Ticket) FeeExpired() bool {
	now := time.Now()
	return now.After(time.Unix(t.FeeExpiration, 0))
}

// InsertNewTicket will insert the provided ticket into the database. Returns an
// error if either the ticket hash or fee address already exist.
func (vdb *VspDatabase) InsertNewTicket(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		// Create a bucket for the new ticket. Returns an error if bucket
		// already exists.
		newTicketBkt, err := ticketBkt.CreateBucket([]byte(ticket.Hash))
		if err != nil {
			return fmt.Errorf("could not create bucket for ticket: %w", err)
		}

		err = putTicketInBucket(newTicketBkt, ticket)
		if err != nil {
			return fmt.Errorf("putting ticket in bucket failed: %w", err)
		}

		return nil
	})
}

// putTicketInBucket encodes each of the fields of the provided ticket as a byte
// array, and stores them as values within the provided db bucket.
func putTicketInBucket(bkt *bolt.Bucket, ticket Ticket) error {
	var err error
	if err = bkt.Put(hashK, []byte(ticket.Hash)); err != nil {
		return err
	}
	if err = bkt.Put(commitmentAddressK, []byte(ticket.CommitmentAddress)); err != nil {
		return err
	}
	if err = bkt.Put(feeAddressK, []byte(ticket.FeeAddress)); err != nil {
		return err
	}
	if err = bkt.Put(votingWIFK, []byte(ticket.VotingWIF)); err != nil {
		return err
	}
	if err = bkt.Put(feeTxHexK, []byte(ticket.FeeTxHex)); err != nil {
		return err
	}
	if err = bkt.Put(feeTxHashK, []byte(ticket.FeeTxHash)); err != nil {
		return err
	}
	if err = bkt.Put(feeTxStatusK, []byte(ticket.FeeTxStatus)); err != nil {
		return err
	}
	if err = bkt.Put(outcomeK, []byte(ticket.Outcome)); err != nil {
		return err
	}
	if err = bkt.Put(purchaseHeightK, int64ToBytes(ticket.PurchaseHeight)); err != nil {
		return err
	}
	if err = bkt.Put(feeAddressIndexK, uint32ToBytes(ticket.FeeAddressIndex)); err != nil {
		return err
	}
	if err = bkt.Put(feeAddressXPubIDK, uint32ToBytes(ticket.FeeAddressXPubID)); err != nil {
		return err
	}
	if err = bkt.Put(feeAmountK, int64ToBytes(ticket.FeeAmount)); err != nil {
		return err
	}
	if err = bkt.Put(feeExpirationK, int64ToBytes(ticket.FeeExpiration)); err != nil {
		return err
	}
	if err = bkt.Put(confirmedK, boolToBytes(ticket.Confirmed)); err != nil {
		return err
	}
	if err = bkt.Put(tSpendPolicyK, stringMapToBytes(ticket.TSpendPolicy)); err != nil {
		return err
	}
	if err = bkt.Put(treasuryPolicyK, stringMapToBytes(ticket.TreasuryPolicy)); err != nil {
		return err
	}

	return bkt.Put(voteChoicesK, stringMapToBytes(ticket.VoteChoices))
}

func getTicketFromBkt(bkt *bolt.Bucket) (Ticket, error) {
	var ticket Ticket

	ticket.Hash = string(bkt.Get(hashK))
	ticket.CommitmentAddress = string(bkt.Get(commitmentAddressK))
	ticket.FeeAddress = string(bkt.Get(feeAddressK))
	ticket.VotingWIF = string(bkt.Get(votingWIFK))
	ticket.FeeTxHex = string(bkt.Get(feeTxHexK))
	ticket.FeeTxHash = string(bkt.Get(feeTxHashK))
	ticket.FeeTxStatus = FeeStatus(bkt.Get(feeTxStatusK))
	ticket.Outcome = TicketOutcome(bkt.Get(outcomeK))

	ticket.PurchaseHeight = bytesToInt64(bkt.Get(purchaseHeightK))
	ticket.FeeAddressXPubID = bytesToUint32(bkt.Get(feeAddressXPubIDK))
	ticket.FeeAddressIndex = bytesToUint32(bkt.Get(feeAddressIndexK))
	ticket.FeeAmount = bytesToInt64(bkt.Get(feeAmountK))
	ticket.FeeExpiration = bytesToInt64(bkt.Get(feeExpirationK))

	ticket.Confirmed = bytesToBool(bkt.Get(confirmedK))

	var err error
	ticket.VoteChoices, err = bytesToStringMap(bkt.Get(voteChoicesK))
	if err != nil {
		return ticket, fmt.Errorf("unmarshal VoteChoices err: %w", err)
	}

	ticket.TSpendPolicy, err = bytesToStringMap(bkt.Get(tSpendPolicyK))
	if err != nil {
		return ticket, fmt.Errorf("unmarshal TSpendPolicy err: %w", err)
	}

	ticket.TreasuryPolicy, err = bytesToStringMap(bkt.Get(treasuryPolicyK))
	if err != nil {
		return ticket, fmt.Errorf("unmarshal TreasuryPolicy err: %w", err)
	}

	return ticket, nil
}

func (vdb *VspDatabase) DeleteTicket(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		err := ticketBkt.DeleteBucket([]byte(ticket.Hash))
		if err != nil {
			return fmt.Errorf("could not delete ticket: %w", err)
		}

		return nil
	})
}

func (vdb *VspDatabase) UpdateTicket(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		bkt := ticketBkt.Bucket([]byte(ticket.Hash))

		if bkt == nil {
			return fmt.Errorf("ticket does not exist with hash %s", ticket.Hash)
		}

		return putTicketInBucket(bkt, ticket)
	})
}

func (vdb *VspDatabase) GetTicketByHash(ticketHash string) (Ticket, bool, error) {
	var ticket Ticket
	var found bool
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK).Bucket([]byte(ticketHash))

		if ticketBkt == nil {
			return nil
		}

		var err error
		ticket, err = getTicketFromBkt(ticketBkt)
		if err != nil {
			return fmt.Errorf("could not get ticket: %w", err)
		}

		found = true

		return nil
	})

	return ticket, found, err
}

// Size returns the current size of the database in bytes. Note that this may
// not exactly match the size of the database file stored on disk.
func (vdb *VspDatabase) Size() (uint64, error) {
	var size uint64
	err := vdb.db.View(func(tx *bolt.Tx) error {
		size = uint64(tx.Size())
		return nil
	})
	return size, err
}

// CountTickets returns the total number of voted, expired, missed, and
// currently voting tickets. This func iterates over every ticket so should be
// used sparingly.
func (vdb *VspDatabase) CountTickets() (int64, int64, int64, int64, error) {
	var voting, voted, expired, missed int64
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEachBucket(func(k []byte) error {
			tBkt := ticketBkt.Bucket(k)

			if FeeStatus(tBkt.Get(feeTxStatusK)) == FeeConfirmed {
				switch TicketOutcome(tBkt.Get(outcomeK)) {
				case Voted:
					voted++
				case Expired:
					expired++
				case Missed:
					missed++
				case Revoked:
					// There shouldn't be any revoked tickets in the db, they
					// should have been updated to expired/missed. Give benefit
					// of doubt to VSP admin and count these as expired.
					expired++
				default:
					voting++
				}
			}

			return nil
		})
	})

	return voting, voted, expired, missed, err
}

// GetUnconfirmedTickets returns tickets which are not yet confirmed.
func (vdb *VspDatabase) GetUnconfirmedTickets() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return !bytesToBool(t.Get(confirmedK))
	})
}

// GetPendingFees returns tickets which are confirmed and have a fee tx which is
// not yet broadcast.
func (vdb *VspDatabase) GetPendingFees() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return bytesToBool(t.Get(confirmedK)) && FeeStatus(t.Get(feeTxStatusK)) == FeeReceieved
	})
}

// GetUnconfirmedFees returns tickets with a fee tx that is broadcast but not
// confirmed yet.
func (vdb *VspDatabase) GetUnconfirmedFees() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return FeeStatus(t.Get(feeTxStatusK)) == FeeBroadcast
	})
}

// GetVotableTickets returns tickets with a confirmed fee tx and no outcome (ie.
// not expired/voted/missed).
func (vdb *VspDatabase) GetVotableTickets() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return FeeStatus(t.Get(feeTxStatusK)) == FeeConfirmed && TicketOutcome(t.Get(outcomeK)) == ""
	})
}

// GetVotedTickets returns tickets with a confirmed fee tx and outcome == voted.
func (vdb *VspDatabase) GetVotedTickets() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return FeeStatus(t.Get(feeTxStatusK)) == FeeConfirmed && TicketOutcome(t.Get(outcomeK)) == Voted
	})
}

// GetRevokedTickets returns all tickets which have outcome == revoked.
func (vdb *VspDatabase) GetRevokedTickets() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return TicketOutcome(t.Get(outcomeK)) == Revoked
	})
}

// GetMissingPurchaseHeight returns tickets which are confirmed but do not have
// a purchase height.
func (vdb *VspDatabase) GetMissingPurchaseHeight() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return bytesToBool(t.Get(confirmedK)) && bytesToInt64(t.Get(purchaseHeightK)) == 0
	})
}

// GetMissedTickets returns all tickets which have outcome == missed.
func (vdb *VspDatabase) GetMissedTickets() (TicketList, error) {
	return vdb.filterTickets(func(t *bolt.Bucket) bool {
		return TicketOutcome(t.Get(outcomeK)) == Missed
	})
}

// filterTickets accepts a filter function and returns all tickets from the
// database which match the filter.
func (vdb *VspDatabase) filterTickets(filter func(*bolt.Bucket) bool) (TicketList, error) {
	var tickets TicketList
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEachBucket(func(k []byte) error {
			ticketBkt := ticketBkt.Bucket(k)

			if filter(ticketBkt) {
				ticket, err := getTicketFromBkt(ticketBkt)
				if err != nil {
					return fmt.Errorf("could not get ticket: %w", err)
				}
				tickets = append(tickets, ticket)
			}

			return nil
		})
	})

	return tickets, err
}
