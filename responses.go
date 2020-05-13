package main

type getPubKeyResponse struct {
	Timestamp int64  `json:"timestamp"`
	PubKey    []byte `json:"pubKey"`
}

type getFeeResponse struct {
	Timestamp int64   `json:"timestamp"`
	Fee       float64 `json:"fee"`
}

type getFeeAddressResponse struct {
	Timestamp           int64  `json:"timestamp"`
	TicketHash          string `json:"ticketHash"`
	CommitmentSignature string `json:"commitmentSignature"`
	FeeAddress          string `json:"feeAddress"`
}

type payFeeResponse struct {
	Timestamp int64  `json:"timestamp"`
	TxHash    string `json:"txHash"`
}
