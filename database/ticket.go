package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Ticket is serialized to json and stored in bbolt db. The json keys are
// deliberately kept short because they are duplicated many times in the db.
type Ticket struct {
	Hash              string  `json:"hsh"`
	CommitmentAddress string  `json:"cmtaddr"`
	FeeAddressIndex   uint32  `json:"faddridx"`
	FeeAddress        string  `json:"faddr"`
	FeeAmount         float64 `json:"famt"`
	FeeExpiration     int64   `json:"fexp"`

	// Confirmed will be set when the ticket has 6+ confirmations.
	Confirmed bool `json:"conf"`

	// VoteChoices and VotingWIF are set in /payfee.
	VoteChoices map[string]string `json:"vchces"`
	VotingWIF   string            `json:"vwif"`

	// FeeTxHex will be set when the fee tx has been received from the user.
	FeeTxHex string `json:"fhex"`

	// FeeTxHash will be set when the fee tx has been broadcast.
	FeeTxHash string `json:"fhsh"`

	// FeeConfirmed will be set when the fee tx has 6+ confirmations.
	FeeConfirmed bool `json:"fconf"`
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

		// Error if a ticket already exists with the same fee address.
		err := ticketBkt.ForEach(func(k, v []byte) error {
			var t Ticket
			err := json.Unmarshal(v, &t)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if t.FeeAddress == ticket.FeeAddress {
				return fmt.Errorf("ticket with fee address %s already exists", t.FeeAddress)
			}

			if t.FeeAddressIndex == ticket.FeeAddressIndex {
				return fmt.Errorf("ticket with fee address index %d already exists", t.FeeAddressIndex)
			}

			return nil
		})
		if err != nil {
			return err
		}

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

			if ticket.FeeConfirmed {
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
