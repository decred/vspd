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
	Timestamp  int64  `json:"timestamp"`
	TicketHash string `json:"ticketHash"`
	Signature  string `json:"signature"`
}

type feeAddressResponse struct {
	Timestamp  int64             `json:"timestamp"`
	FeeAddress string            `json:"feeAddress"`
	Fee        float64           `json:"fee"`
	Expiration int64             `json:"expiration"`
	Request    FeeAddressRequest `json:"request"`
}

type PayFeeRequest struct {
	Timestamp int64  `json:"timestamp"`
	Hex       []byte `json:"feeTx"`
	VotingKey string `json:"votingKey"`
	VoteBits  uint16 `json:"voteBits"`
}

type payFeeResponse struct {
	Timestamp int64         `json:"timestamp"`
	TxHash    string        `json:"txHash"`
	Request   PayFeeRequest `json:"request"`
}
