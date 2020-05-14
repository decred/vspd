package main

type pubKeyResponse struct {
	Timestamp int64  `json:"timestamp"`
	PubKey    []byte `json:"pubKey"`
}

type feeResponse struct {
	Timestamp int64   `json:"timestamp"`
	Fee       float64 `json:"fee"`
}

type feeAddressResponse struct {
	Timestamp           int64  `json:"timestamp"`
	TicketHash          string `json:"ticketHash"`
	CommitmentSignature string `json:"commitmentSignature"`
	FeeAddress          string `json:"feeAddress"`
}

type payFeeResponse struct {
	Timestamp int64  `json:"timestamp"`
	TxHash    string `json:"txHash"`
}
