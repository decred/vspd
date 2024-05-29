// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

type ErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e ErrorResponse) Error() string { return e.Message }

type VspInfoResponse struct {
	APIVersions         []int64 `json:"apiversions"`
	Timestamp           int64   `json:"timestamp"`
	PubKey              []byte  `json:"pubkey"`
	FeePercentage       float64 `json:"feepercentage"`
	VspClosed           bool    `json:"vspclosed"`
	VspClosedMsg        string  `json:"vspclosedmsg"`
	Network             string  `json:"network"`
	VspdVersion         string  `json:"vspdversion"`
	Voting              int64   `json:"voting"`
	Voted               int64   `json:"voted"`
	TotalVotingWallets  int64   `json:"totalvotingwallets"`
	VotingWalletsOnline int64   `json:"votingwalletsonline"`
	Expired             int64   `json:"expired"`
	Missed              int64   `json:"missed"`
	BlockHeight         uint32  `json:"blockheight"`
	NetworkProportion   float32 `json:"estimatednetworkproportion"`
}

type FeeAddressRequest struct {
	Timestamp  int64  `json:"timestamp" binding:"required"`
	TicketHash string `json:"tickethash" binding:"required"`
	TicketHex  string `json:"tickethex" binding:"required"`
	ParentHex  string `json:"parenthex" binding:"required"`
}

type FeeAddressResponse struct {
	Timestamp  int64  `json:"timestamp"`
	FeeAddress string `json:"feeaddress"`
	FeeAmount  int64  `json:"feeamount"`
	Expiration int64  `json:"expiration"`
	Request    []byte `json:"request"`
}

type PayFeeRequest struct {
	Timestamp      int64             `json:"timestamp" binding:"required"`
	TicketHash     string            `json:"tickethash" binding:"required"`
	FeeTx          string            `json:"feetx" binding:"required"`
	VotingKey      string            `json:"votingkey" binding:"required"`
	VoteChoices    map[string]string `json:"votechoices" binding:"required"`
	TSpendPolicy   map[string]string `json:"tspendpolicy" binding:"max=3"`
	TreasuryPolicy map[string]string `json:"treasurypolicy" binding:"max=3"`
}

type PayFeeResponse struct {
	Timestamp int64  `json:"timestamp"`
	Request   []byte `json:"request"`
}

type SetVoteChoicesRequest struct {
	Timestamp      int64             `json:"timestamp" binding:"required"`
	TicketHash     string            `json:"tickethash" binding:"required"`
	VoteChoices    map[string]string `json:"votechoices" binding:"required"`
	TSpendPolicy   map[string]string `json:"tspendpolicy" binding:"max=3"`
	TreasuryPolicy map[string]string `json:"treasurypolicy" binding:"max=3"`
}

type SetVoteChoicesResponse struct {
	Timestamp int64  `json:"timestamp"`
	Request   []byte `json:"request"`
}

type TicketStatusRequest struct {
	TicketHash string `json:"tickethash" binding:"required"`
}

type TicketStatusResponse struct {
	Timestamp       int64             `json:"timestamp"`
	TicketConfirmed bool              `json:"ticketconfirmed"`
	FeeTxStatus     string            `json:"feetxstatus"`
	FeeTxHash       string            `json:"feetxhash"`
	AltSignAddress  string            `json:"altsignaddress"`
	VoteChoices     map[string]string `json:"votechoices"`
	TSpendPolicy    map[string]string `json:"tspendpolicy"`
	TreasuryPolicy  map[string]string `json:"treasurypolicy"`
	Request         []byte            `json:"request"`
}

type SetAltSignAddrRequest struct {
	Timestamp      int64  `json:"timestamp" binding:"required"`
	TicketHash     string `json:"tickethash" binding:"required"`
	TicketHex      string `json:"tickethex" binding:"required"`
	ParentHex      string `json:"parenthex" binding:"required"`
	AltSignAddress string `json:"altsignaddress" binding:"required"`
}

type SetAltSignAddrResponse struct {
	Timestamp int64  `json:"timestamp"`
	Request   []byte `json:"request"`
}
