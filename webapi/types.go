package webapi

type vspInfoResponse struct {
	Timestamp     int64   `json:"timestamp"`
	PubKey        []byte  `json:"pubkey"`
	FeePercentage float64 `json:"feepercentage"`
	VspClosed     bool    `json:"vspclosed"`
	Network       string  `json:"network"`
	VspdVersion   string  `json:"vspdversion"`
}

type FeeAddressRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	TicketHex  string `json:"tickethex" binding:"required"`
}

type feeAddressResponse struct {
	Timestamp  int64             `json:"timestamp"`
	FeeAddress string            `json:"feeaddress"`
	FeeAmount  int64             `json:"feeamount"`
	Expiration int64             `json:"expiration"`
	Request    FeeAddressRequest `json:"request"`
}

type PayFeeRequest struct {
	Timestamp   int64             `json:"timestamp" binding:"required"`
	TicketHash  string            `json:"tickethash" binding:"required"`
	FeeTx       string            `json:"feetx" binding:"required"`
	VotingKey   string            `json:"votingkey" binding:"required"`
	VoteChoices map[string]string `json:"votechoices" binding:"required"`
}

type payFeeResponse struct {
	Timestamp int64         `json:"timestamp"`
	Request   PayFeeRequest `json:"request"`
}

type SetVoteChoicesRequest struct {
	Timestamp   int64             `json:"timestamp" binding:"required"`
	TicketHash  string            `json:"tickethash" binding:"required"`
	VoteChoices map[string]string `json:"votechoices" binding:"required"`
}

type setVoteChoicesResponse struct {
	Timestamp int64                 `json:"timestamp"`
	Request   SetVoteChoicesRequest `json:"request"`
}

type TicketStatusRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
}

type ticketStatusResponse struct {
	Timestamp       int64               `json:"timestamp"`
	TicketConfirmed bool                `json:"ticketconfirmed"`
	FeeTxStatus     string              `json:"feetxstatus"`
	FeeTxHash       string              `json:"feetxhash"`
	VoteChoices     map[string]string   `json:"votechoices"`
	Request         TicketStatusRequest `json:"request"`
}
