package webapi

type vspInfoResponse struct {
	APIVersions   []int64 `json:"apiversions"`
	Timestamp     int64   `json:"timestamp"`
	PubKey        []byte  `json:"pubkey"`
	FeePercentage float64 `json:"feepercentage"`
	VspClosed     bool    `json:"vspclosed"`
	Network       string  `json:"network"`
	VspdVersion   string  `json:"vspdversion"`
	Voting        int64   `json:"voting"`
	Voted         int64   `json:"voted"`
	Revoked       int64   `json:"revoked"`
}

type feeAddressRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	TicketHex  string `json:"tickethex" binding:"required"`
}

type feeAddressResponse struct {
	Timestamp  int64             `json:"timestamp"`
	FeeAddress string            `json:"feeaddress"`
	FeeAmount  int64             `json:"feeamount"`
	Expiration int64             `json:"expiration"`
	Request    feeAddressRequest `json:"request"`
}

type payFeeRequest struct {
	Timestamp   int64             `json:"timestamp" binding:"required"`
	TicketHash  string            `json:"tickethash" binding:"required"`
	FeeTx       string            `json:"feetx" binding:"required"`
	VotingKey   string            `json:"votingkey" binding:"required"`
	VoteChoices map[string]string `json:"votechoices" binding:"required"`
}

type payFeeResponse struct {
	Timestamp int64         `json:"timestamp"`
	Request   payFeeRequest `json:"request"`
}

type setVoteChoicesRequest struct {
	Timestamp   int64             `json:"timestamp" binding:"required"`
	TicketHash  string            `json:"tickethash" binding:"required"`
	VoteChoices map[string]string `json:"votechoices" binding:"required"`
}

type setVoteChoicesResponse struct {
	Timestamp int64                 `json:"timestamp"`
	Request   setVoteChoicesRequest `json:"request"`
}

type ticketStatusRequest struct {
	TicketHash string `json:"tickethash" binding:"required"`
}

type ticketStatusResponse struct {
	Timestamp       int64               `json:"timestamp"`
	TicketConfirmed bool                `json:"ticketconfirmed"`
	FeeTxStatus     string              `json:"feetxstatus"`
	FeeTxHash       string              `json:"feetxhash"`
	VoteChoices     map[string]string   `json:"votechoices"`
	Request         ticketStatusRequest `json:"request"`
}
