package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// TODO: Properly document ticket lifecycle.
// TODO: Shorten json keys, they are stored in the db and duplicated many times.

type Ticket struct {
	Hash              string  `json:"hash"`
	CommitmentAddress string  `json:"commitmentaddress"`
	FeeAddressIndex   uint32  `json:"feeaddressindex"`
	FeeAddress        string  `json:"feeaddress"`
	FeeAmount         float64 `json:"feeamount"`
	FeeExpiration     int64   `json:"feeexpiration"`

	// Confirmed will be set when the ticket has 6+ confirmations.
	Confirmed bool `json:"confirmed"`

	// VoteChoices and VotingWIF are set in /payfee.
	VoteChoices map[string]string `json:"votechoices"`
	VotingWIF   string            `json:"votingwif"`

	// FeeTxHex will be set when the fee tx has been received from the user.
	FeeTxHex string `json:"feetxhex"`

	// FeeTxHash will be set when the fee tx has been broadcast.
	FeeTxHash string `json:"feetxhash"`

	// FeeConfirmed will be set when the fee tx has 6+ confirmations.
	FeeConfirmed bool `json:"feeconfirmed"`
}

func (t *Ticket) FeeExpired() bool {
	now := time.Now()
	return now.After(time.Unix(t.FeeExpiration, 0))
}

var (
	ErrNoTicketFound = errors.New("no ticket found")
)

func (vdb *VspDatabase) InsertNewTicket(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		hashBytes := []byte(ticket.Hash)

		if ticketBkt.Get(hashBytes) != nil {
			return fmt.Errorf("ticket already exists with hash %s", ticket.Hash)
		}

		// TODO: Error if a ticket already exists with the same fee address.

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) UpdateTicket(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		hashBytes := []byte(ticket.Hash)

		if ticketBkt.Get(hashBytes) == nil {
			return fmt.Errorf("ticket does not exist with hash %s", ticket.Hash)
		}

		// TODO: Error if a ticket already exists with the same fee address.

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) GetTicketByHash(ticketHash string) (Ticket, bool, error) {
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
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}

		found = true

		return nil
	})

	return ticket, found, err
}

func (vdb *VspDatabase) CountTickets() (int, int, error) {
	var total, feePaid int
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			total++
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if ticket.FeeTxHash != "" {
				feePaid++
			}

			return nil
		})
	})

	return total, feePaid, err
}

func (vdb *VspDatabase) GetUnconfirmedTickets() ([]Ticket, error) {
	var tickets []Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if !ticket.Confirmed {
				tickets = append(tickets, ticket)
			}

			return nil
		})
	})

	return tickets, err
}

func (vdb *VspDatabase) GetPendingFees() ([]Ticket, error) {
	var tickets []Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			// Add ticket if it is confirmed, and we have a fee tx, and the tx
			// is not broadcast yet.
			if ticket.Confirmed &&
				ticket.FeeTxHex != "" &&
				ticket.FeeTxHash == "" {
				tickets = append(tickets, ticket)
			}

			return nil
		})
	})

	return tickets, err
}

func (vdb *VspDatabase) GetUnconfirmedFees() ([]Ticket, error) {
	var tickets []Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		return ticketBkt.ForEach(func(k, v []byte) error {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			// Add ticket if fee tx is broadcast but not confirmed yet.
			if ticket.FeeTxHash != "" &&
				!ticket.FeeConfirmed {
				tickets = append(tickets, ticket)
			}

			return nil
		})
	})

	return tickets, err
}
