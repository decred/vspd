package database

import (
	"encoding/json"
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

type Ticket struct {
	Hash                string            `json:"hash"`
	CommitmentSignature string            `json:"commitmentsignature"`
	CommitmentAddress   string            `json:"commitmentaddress"`
	FeeAddress          string            `json:"feeaddress"`
	SDiff               float64           `json:"sdiff"`
	BlockHeight         int64             `json:"blockheight"`
	VoteChoices         map[string]string `json:"votechoices"`
	VotingKey           string            `json:"votingkey"`
	VSPFee              float64           `json:"vspfee"`
	Expiration          int64             `json:"expiration"`
}

var (
	ErrNoTicketFound = errors.New("no ticket found")
)

func (vdb *VspDatabase) InsertFeeAddress(ticket Ticket) error {
	hashBytes := []byte(ticket.Hash)
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		if ticketBkt.Get(hashBytes) != nil {
			return fmt.Errorf("ticket already exists with hash %s", ticket.Hash)
		}

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return err
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) InsertFeeAddressVotingKey(address, votingKey string, voteChoices map[string]string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		c := ticketBkt.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if ticket.CommitmentAddress == address {
				ticket.VotingKey = votingKey
				ticket.VoteChoices = voteChoices
				ticketBytes, err := json.Marshal(ticket)
				if err != nil {
					return err
				}
				err = ticketBkt.Put(k, ticketBytes)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (vdb *VspDatabase) GetTicketByHash(hash string) (Ticket, error) {
	var ticket Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		ticketBytes := ticketBkt.Get([]byte(hash))
		if ticketBytes == nil {
			return ErrNoTicketFound
		}

		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}

		return nil
	})

	return ticket, err
}

func (vdb *VspDatabase) UpdateVoteChoices(hash string, voteChoices map[string]string) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		key := []byte(hash)

		ticketBytes := ticketBkt.Get(key)
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

		return ticketBkt.Put(key, ticketBytes)
	})
}

func (vdb *VspDatabase) UpdateExpireAndFee(hash string, expiration int64, vspFee float64) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		key := []byte(hash)

		ticketBytes := ticketBkt.Get(key)
		if ticketBytes == nil {
			return ErrNoTicketFound
		}

		var ticket Ticket
		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}
		ticket.Expiration = expiration
		ticket.VSPFee = vspFee

		ticketBytes, err = json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(key, ticketBytes)
	})
}
