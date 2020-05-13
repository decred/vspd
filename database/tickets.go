package database

type Ticket struct {
	Hash                string
	CommitmentSignature string
	FeeAddress          string
	Address             string
	SDiff               int64
	BlockHeight         int64
	VoteBits            uint16
	VotingKey           string
}

func (db *VspDatabase) InsertFeeAddressVotingKey(address, votingKey string, voteBits uint16) error {
	return nil
}

func (db *VspDatabase) InsertFeeAddress(t Ticket) error {
	return nil
}

func (db *VspDatabase) GetInactiveFeeAddresses() ([]string, error) {
	return []string{""}, nil
}

func (db *VspDatabase) GetFeesByFeeAddress(feeAddr string) (Ticket, error) {
	return Ticket{}, nil
}

func (db *VspDatabase) GetFeeAddressByTicketHash() (Ticket, error) {
	return Ticket{}, nil
}
