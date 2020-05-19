package webapi

type pubKeyResponse struct {
	Timestamp int64  `json:"timestamp" binding:"required"`
	PubKey    []byte `json:"pubKey" binding:"required"`
}

type feeResponse struct {
	Timestamp int64   `json:"timestamp" binding:"required"`
	Fee       float64 `json:"fee" binding:"required"`
}

type FeeAddressRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"ticketHash" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

type feeAddressResponse struct {
	Timestamp  int64             `json:"timestamp" binding:"required"`
	FeeAddress string            `json:"feeAddress" binding:"required"`
	Fee        float64           `json:"fee" binding:"required"`
	Expiration int64             `json:"expiration" binding:"required"`
	Request    FeeAddressRequest `json:"request" binding:"required"`
}

type PayFeeRequest struct {
	Timestamp int64  `json:"timestamp" binding:"required"`
	Hex       string `json:"feeTx" binding:"required"`
	VotingKey string `json:"votingKey" binding:"required"`
	VoteBits  uint16 `json:"voteBits" binding:"required"`
}

type payFeeResponse struct {
	Timestamp int64         `json:"timestamp" binding:"required"`
	TxHash    string        `json:"txHash" binding:"required"`
	Request   PayFeeRequest `json:"request" binding:"required"`
}

type SetVoteBitsRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"ticketHash" binding:"required"`
	Signature  string `json:"commitmentSignature" binding:"required"`
	VoteBits   uint16 `json:"voteBits" binding:"required"`
}

type setVoteBitsResponse struct {
	Timestamp int64              `json:"timestamp" binding:"required"`
	Request   SetVoteBitsRequest `json:"request" binding:"required"`
	VoteBits  uint16             `json:"voteBits" binding:"required"`
}

type TicketStatusRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"ticketHash" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

type ticketStatusResponse struct {
	Timestamp int64               `json:"timestamp" binding:"required"`
	Request   TicketStatusRequest `json:"request" binding:"required"`
	Status    string              `json:"status" binding:"required"`
	VoteBits  uint16              `json:"votebits" binding:"required"`
}
