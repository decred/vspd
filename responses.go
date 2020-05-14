package main

type pubKeyResponse struct {
	Timestamp int64  `json:"timestamp"`
	PubKey    []byte `json:"pubKey"`
}

type feeResponse struct {
	Timestamp int64   `json:"timestamp"`
	Fee       float64 `json:"fee"`
}

type FeeAddressRequest struct {
	TicketHash string `json:"ticketHash"`
	Signature  string `json:"signature"`
}

type feeAddressResponse struct {
	Timestamp           int64  `json:"timestamp"`
	TicketHash          string `json:"ticketHash"`
	CommitmentSignature string `json:"commitmentSignature"`
	FeeAddress          string `json:"feeAddress"`
	Expiration          int64  `json:"expiration"`
}

type PayFeeRequest struct {
	Hex       []byte `json:"feeTx"`
	VotingKey string `json:"votingKey"`
	VoteBits  uint16 `json:"voteBits"`
}

type payFeeResponse struct {
	Timestamp int64  `json:"timestamp"`
	TxHash    string `json:"txHash"`
}
