package webapi

type pubKeyResponse struct {
	Timestamp int64  `json:"timestamp" binding:"required"`
	PubKey    []byte `json:"pubkey" binding:"required"`
}

type feeResponse struct {
	Timestamp int64   `json:"timestamp" binding:"required"`
	Fee       float64 `json:"fee" binding:"required"`
}

type FeeAddressRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

type feeAddressResponse struct {
	Timestamp  int64             `json:"timestamp" binding:"required"`
	FeeAddress string            `json:"feeaddress" binding:"required"`
	Fee        float64           `json:"fee" binding:"required"`
	Expiration int64             `json:"expiration" binding:"required"`
	Request    FeeAddressRequest `json:"request" binding:"required"`
}

type PayFeeRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	FeeTx      string `json:"feetx" binding:"required"`
	VotingKey  string `json:"votingkey" binding:"required"`
	VoteBits   uint16 `json:"votebits" binding:"required"`
}

type payFeeResponse struct {
	Timestamp int64         `json:"timestamp" binding:"required"`
	TxHash    string        `json:"txhash" binding:"required"`
	Request   PayFeeRequest `json:"request" binding:"required"`
}

type SetVoteBitsRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	Signature  string `json:"commitmentsignature" binding:"required"`
	VoteBits   uint16 `json:"votebits" binding:"required"`
}

type setVoteBitsResponse struct {
	Timestamp int64              `json:"timestamp" binding:"required"`
	Request   SetVoteBitsRequest `json:"request" binding:"required"`
	VoteBits  uint16             `json:"votebits" binding:"required"`
}

type TicketStatusRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	Signature  string `json:"signature" binding:"required"`
}

type ticketStatusResponse struct {
	Timestamp int64               `json:"timestamp" binding:"required"`
	Request   TicketStatusRequest `json:"request" binding:"required"`
	Status    string              `json:"status" binding:"required"`
	VoteBits  uint16              `json:"votebits" binding:"required"`
}
