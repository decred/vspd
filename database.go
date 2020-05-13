package main

type Database struct {
}

type Fees struct {
	TicketHash          string
	CommitmentSignature string
	FeeAddress          string
	Address             string
	SDiff               int64
	BlockHeight         int64
	VoteBits            uint16
	VotingKey           string
}

func (db *Database) GetInactiveFeeAddresses() ([]string, error) {
	return []string{""}, nil
}

func (db *Database) GetFeesByFeeAddress(feeAddr string) (Fees, error) {
	return Fees{}, nil
}

func (db *Database) InsertFeeAddressVotingKey(address, votingKey string, voteBits uint16) error {
	return nil
}
