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

func (vdb *VspDatabase) InsertTicket(ticket Ticket) error {
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

func (vdb *VspDatabase) SetTicketVotingKey(ticketHash, votingKey string, voteChoices map[string]string) error {
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
		ticketBytes, err = json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}

func (vdb *VspDatabase) GetTicketByHash(ticketHash string) (Ticket, error) {
	var ticket Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		ticketBytes := ticketBkt.Get([]byte(ticketHash))
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
		ticket.Expiration = expiration
		ticket.VSPFee = vspFee

		ticketBytes, err = json.Marshal(ticket)
		if err != nil {
			return fmt.Errorf("could not marshal ticket: %v", err)
		}

		return ticketBkt.Put(hashBytes, ticketBytes)
	})
}
