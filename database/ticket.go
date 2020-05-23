package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type Ticket struct {
	Hash              string            `json:"hash"`
	CommitmentAddress string            `json:"commitmentaddress"`
	FeeAddress        string            `json:"feeaddress"`
	SDiff             float64           `json:"sdiff"`
	BlockHeight       int64             `json:"blockheight"`
	VoteChoices       map[string]string `json:"votechoices"`
	VotingKey         string            `json:"votingkey"`
	VSPFee            float64           `json:"vspfee"`
	FeeExpiration     int64             `json:"feeexpiration"`
	FeeTxHash         string            `json:"feetxhash"`
}

func (t *Ticket) FeeExpired() bool {
	now := time.Now()
	return now.After(time.Unix(t.FeeExpiration, 0))
}

var (
	ErrNoTicketFound = errors.New("no ticket found")
)

func (vdb *VspDatabase) InsertTicket(ticket Ticket) error {
	hashBytes := []byte(ticket.Hash)
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		if ticketBkt.Get(hashBytes) != nil {
			return fmt.Errorf("ticket already exists with hash %s", ticket.Hash)
		}

		// TODO: Error if a ticket already exists with the same fee address.

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return err
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) SetTicketVotingKey(ticketHash, votingKey string, voteChoices map[string]string, feeTxHash string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		hashBytes := []byte(ticketHash)

		ticketBytes := ticketBkt.Get(hashBytes)
		if ticketBytes == nil {
			return ErrNoTicketFound
		}

		var ticket Ticket
		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}

		ticket.VotingKey = votingKey
		ticket.VoteChoices = voteChoices
		ticket.FeeTxHash = feeTxHash

		ticketBytes, err = json.Marshal(ticket)
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

func (vdb *VspDatabase) UpdateVoteChoices(ticketHash string, voteChoices map[string]string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		hashBytes := []byte(ticketHash)

		ticketBytes := ticketBkt.Get(hashBytes)
		if ticketBytes == nil {
			return ErrNoTicketFound
		}

		var ticket Ticket
		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}
		ticket.VoteChoices = voteChoices

		ticketBytes, err = json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) UpdateExpireAndFee(ticketHash string, expiration int64, vspFee float64) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		hashBytes := []byte(ticketHash)

		ticketBytes := ticketBkt.Get(hashBytes)
		if ticketBytes == nil {
			return ErrNoTicketFound
		}

		var ticket Ticket
		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}
		ticket.FeeExpiration = expiration
		ticket.VSPFee = vspFee

		ticketBytes, err = json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
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
