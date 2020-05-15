package database

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

type Ticket struct {
	Hash                string `json:"hash"`
	CommitmentSignature string `json:"commitmentsignature"`
	FeeAddress          string `json:"feeaddress"`
	Address             string `json:"address"`
	SDiff               int64  `json:"sdiff"`
	BlockHeight         int64  `json:"blockheight"`
	VoteBits            uint16 `json:"votebits"`
	VotingKey           string `json:"votingkey"`
}

func (vdb *VspDatabase) InsertFeeAddress(ticket Ticket) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		if ticketBkt.Get([]byte(ticket.Hash)) != nil {
			return fmt.Errorf("ticket already exists with hash %s", ticket.Hash)
		}

		ticketBytes, err := json.Marshal(ticket)
		if err != nil {
			return err
		}

		return ticketBkt.Put([]byte(ticket.Hash), ticketBytes)
	})
}

func (vdb *VspDatabase) InsertFeeAddressVotingKey(address, votingKey string, voteBits uint16) error {
	return vdb.db.Update(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		c := ticketBkt.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if ticket.Address == address {
				ticket.VotingKey = votingKey
				ticket.VoteBits = voteBits
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

func (vdb *VspDatabase) GetInactiveFeeAddresses() ([]string, error) {
	var addrs []string
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		c := ticketBkt.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if ticket.VotingKey == "" {
				addrs = append(addrs, ticket.FeeAddress)
			}
		}

		return nil
	})

	return addrs, err
}

func (vdb *VspDatabase) GetFeesByFeeAddress(feeAddr string) (*Ticket, error) {
	var tickets []Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)
		c := ticketBkt.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %v", err)
			}

			if ticket.FeeAddress == feeAddr {
				tickets = append(tickets, ticket)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(tickets) != 1 {
		return nil, fmt.Errorf("expected 1 ticket with fee address %s, found %d", feeAddr, len(tickets))
	}

	return &tickets[0], nil
}

func (vdb *VspDatabase) GetFeeAddressByTicketHash(ticketHash string) (Ticket, error) {
	var ticket Ticket
	err := vdb.db.View(func(tx *bolt.Tx) error {
		ticketBkt := tx.Bucket(vspBktK).Bucket(ticketBktK)

		ticketBytes := ticketBkt.Get([]byte(ticketHash))
		if ticketBytes == nil {
			return fmt.Errorf("no ticket found with hash %s", ticketHash)
		}

		err := json.Unmarshal(ticketBytes, &ticket)
		if err != nil {
			return fmt.Errorf("could not unmarshal ticket: %v", err)
		}

		return nil
	})

	return ticket, err
}
