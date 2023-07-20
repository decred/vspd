// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"encoding/json"
	"net/http"

	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// WalletStatus describes the current status of a single voting wallet. This is
// used by the admin.html template, and also serialized to JSON for the
// /admin/status endpoint.
type WalletStatus struct {
	Connected       bool   `json:"connected"`
	InfoError       bool   `json:"infoerror"`
	DaemonConnected bool   `json:"daemonconnected"`
	VoteVersion     uint32 `json:"voteversion"`
	Unlocked        bool   `json:"unlocked"`
	Voting          bool   `json:"voting"`
	BestBlockError  bool   `json:"bestblockerror"`
	BestBlockHeight int64  `json:"bestblockheight"`
}

// DcrdStatus describes the current status of the local instance of dcrd used by
// vspd. This is used by the admin.html template, and also serialized to JSON
// for the /admin/status endpoint.
type DcrdStatus struct {
	Host            string `json:"host"`
	Connected       bool   `json:"connected"`
	BestBlockError  bool   `json:"bestblockerror"`
	BestBlockHeight uint32 `json:"bestblockheight"`
}

type searchResult struct {
	Hash            string
	Found           bool
	Ticket          database.Ticket
	FeeTxDecoded    string
	AltSignAddrData *database.AltSignAddrData
	VoteChanges     map[uint32]database.VoteChangeRecord
	MaxVoteChanges  int
}

func (s *Server) dcrdStatus(c *gin.Context) DcrdStatus {
	hostname := c.MustGet(dcrdHostKey).(string)
	status := DcrdStatus{Host: hostname}

	dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		s.log.Errorf("Could not get dcrd client: %v", dcrdErr.(error))
		return status
	}

	status.Connected = true

	bestBlock, err := dcrdClient.GetBestBlockHeader()
	if err != nil {
		s.log.Errorf("Could not get dcrd best block header: %v", err)
		status.BestBlockError = true
		return status
	}

	status.BestBlockHeight = bestBlock.Height

	return status
}

func (s *Server) walletStatus(c *gin.Context) map[string]WalletStatus {
	walletClients := c.MustGet(walletsKey).([]*rpc.WalletRPC)
	failedWalletClients := c.MustGet(failedWalletsKey).([]string)

	walletStatus := make(map[string]WalletStatus)
	for _, v := range walletClients {
		ws := WalletStatus{Connected: true}

		walletInfo, err := v.WalletInfo()
		if err != nil {
			s.log.Errorf("dcrwallet.WalletInfo error (wallet=%s): %v", v.String(), err)
			ws.InfoError = true
		} else {
			ws.DaemonConnected = walletInfo.DaemonConnected
			ws.VoteVersion = walletInfo.VoteVersion
			ws.Unlocked = walletInfo.Unlocked
			ws.Voting = walletInfo.Voting
		}

		height, err := v.GetBestBlockHeight()
		if err != nil {
			s.log.Errorf("dcrwallet.GetBestBlockHeight error (wallet=%s): %v", v.String(), err)
			ws.BestBlockError = true
		} else {
			ws.BestBlockHeight = height
		}

		walletStatus[v.String()] = ws
	}
	for _, v := range failedWalletClients {
		ws := WalletStatus{Connected: false}
		walletStatus[v] = ws
	}
	return walletStatus
}

// statusJSON is the handler for "GET /admin/status". It returns a JSON object
// describing the current status of voting wallets.
func (s *Server) statusJSON(c *gin.Context) {
	httpStatus := http.StatusOK

	wallets := s.walletStatus(c)

	// Respond with HTTP status 500 if any voting wallets have issues.
	for _, wallet := range wallets {
		if wallet.InfoError ||
			wallet.BestBlockError ||
			!wallet.Connected ||
			!wallet.DaemonConnected ||
			!wallet.Voting ||
			!wallet.Unlocked {
			httpStatus = http.StatusInternalServerError
			break
		}
	}

	dcrd := s.dcrdStatus(c)

	// Respond with HTTP status 500 if dcrd has issues.
	if !dcrd.Connected || dcrd.BestBlockError {
		httpStatus = http.StatusInternalServerError
	}

	c.AbortWithStatusJSON(httpStatus, gin.H{
		"wallets": wallets,
		"dcrd":    dcrd,
	})
}

// adminPage is the handler for "GET /admin".
func (s *Server) adminPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin.html", gin.H{
		"WebApiCache":  s.cache.getData(),
		"WebApiCfg":    s.cfg,
		"WalletStatus": s.walletStatus(c),
		"DcrdStatus":   s.dcrdStatus(c),
	})
}

// ticketSearch is the handler for "POST /admin/ticket". The hash param will be
// used to retrieve a ticket from the database.
func (s *Server) ticketSearch(c *gin.Context) {
	hash := c.PostForm("hash")

	ticket, found, err := s.db.GetTicketByHash(hash)
	if err != nil {
		s.log.Errorf("db.GetTicketByHash error (ticketHash=%s): %v", hash, err)
		c.String(http.StatusInternalServerError, "Error getting ticket from db")
		return
	}

	voteChanges, err := s.db.GetVoteChanges(hash)
	if err != nil {
		s.log.Errorf("db.GetVoteChanges error (ticketHash=%s): %v", hash, err)
		c.String(http.StatusInternalServerError, "Error getting vote changes from db")
		return
	}

	altSignAddrData, err := s.db.AltSignAddrData(hash)
	if err != nil {
		s.log.Errorf("db.AltSignAddrData error (ticketHash=%s): %v", hash, err)
		c.String(http.StatusInternalServerError, "Error getting alt sig from db")
		return
	}

	// Decode the fee tx so it can be displayed human-readable. Fee tx hex may
	// be null because it is removed from the DB if the tx is already mined and
	// confirmed.
	var feeTxDecoded string
	if ticket.FeeTxHex != "" {
		dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
		dcrdErr := c.MustGet(dcrdErrorKey)
		if dcrdErr != nil {
			s.log.Errorf("Could not get dcrd client: %v", dcrdErr.(error))
			c.String(http.StatusInternalServerError, "Could not get dcrd client")
			return
		}

		resp, err := dcrdClient.DecodeRawTransaction(ticket.FeeTxHex)
		if err != nil {
			s.log.Errorf("dcrd.DecodeRawTransaction error: %w", err)
			c.String(http.StatusInternalServerError, "Error decoding fee transaction")
			return
		}

		decoded, err := json.Marshal(resp)
		if err != nil {
			s.log.Errorf("Unmarshal fee tx error: %w", err)
			c.String(http.StatusInternalServerError, "Error unmarshalling fee tx")
			return
		}

		feeTxDecoded = string(decoded)
	}

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"SearchResult": searchResult{
			Hash:            hash,
			Found:           found,
			Ticket:          ticket,
			FeeTxDecoded:    feeTxDecoded,
			AltSignAddrData: altSignAddrData,
			VoteChanges:     voteChanges,
			MaxVoteChanges:  s.cfg.MaxVoteChangeRecords,
		},
		"WebApiCache":  s.cache.getData(),
		"WebApiCfg":    s.cfg,
		"WalletStatus": s.walletStatus(c),
		"DcrdStatus":   s.dcrdStatus(c),
	})
}

// adminLogin is the handler for "POST /admin". If a valid password is provided,
// the current session will be authenticated as an admin.
func (s *Server) adminLogin(c *gin.Context) {
	password := c.PostForm("password")

	if password != s.cfg.AdminPass {
		s.log.Warnf("Failed login attempt from %s", c.ClientIP())
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"WebApiCache":    s.cache.getData(),
			"WebApiCfg":      s.cfg,
			"FailedLoginMsg": "Incorrect password",
		})
		return
	}

	s.setAdminStatus(true, c)
}

// adminLogout is the handler for "POST /admin/logout". The current session will
// have its admin authentication removed.
func (s *Server) adminLogout(c *gin.Context) {
	s.setAdminStatus(nil, c)
}

// downloadDatabaseBackup is the handler for "GET /backup". A binary
// representation of the whole database is generated and returned to the client.
func (s *Server) downloadDatabaseBackup(c *gin.Context) {
	err := s.db.BackupDB(c.Writer)
	if err != nil {
		s.log.Errorf("Error backing up database: %v", err)
		// Don't write any http body here because Content-Length has already
		// been set in db.BackupDB. Status is enough to indicate an error.
		c.Status(http.StatusInternalServerError)
	}
}

// setAdminStatus stores the authentication status of the current session and
// redirects the client to GET /admin.
func (s *Server) setAdminStatus(admin any, c *gin.Context) {
	session := c.MustGet(sessionKey).(*sessions.Session)
	session.Values["admin"] = admin
	err := session.Save(c.Request, c.Writer)
	if err != nil {
		s.log.Errorf("Error saving session: %v", err)
		c.String(http.StatusInternalServerError, "Error saving session")
		return
	}

	c.Redirect(http.StatusFound, "/admin")
	c.Abort()
}
