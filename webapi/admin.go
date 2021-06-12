// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
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

type searchResult struct {
	Hash           string
	Found          bool
	Ticket         database.Ticket
	VoteChanges    map[uint32]database.VoteChangeRecord
	MaxVoteChanges int
}

func walletStatus(c *gin.Context) map[string]WalletStatus {
	walletClients := c.MustGet("WalletClients").([]*rpc.WalletRPC)
	failedWalletClients := c.MustGet("FailedWalletClients").([]string)

	walletStatus := make(map[string]WalletStatus)
	for _, v := range walletClients {
		ws := WalletStatus{Connected: true}

		walletInfo, err := v.WalletInfo()
		if err != nil {
			log.Errorf("dcrwallet.WalletInfo error (wallet=%s): %v", v.String(), err)
			ws.InfoError = true
		} else {
			ws.DaemonConnected = walletInfo.DaemonConnected
			ws.VoteVersion = walletInfo.VoteVersion
			ws.Unlocked = walletInfo.Unlocked
			ws.Voting = walletInfo.Voting
		}

		height, err := v.GetBestBlockHeight()
		if err != nil {
			log.Errorf("dcrwallet.GetBestBlockHeight error (wallet=%s): %v", v.String(), err)
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
func statusJSON(c *gin.Context) {
	httpStatus := http.StatusOK

	wallets := walletStatus(c)

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

	c.AbortWithStatusJSON(httpStatus, wallets)
}

// adminPage is the handler for "GET /admin".
func adminPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin.html", gin.H{
		"WebApiCache":  getCache(),
		"WebApiCfg":    cfg,
		"WalletStatus": walletStatus(c),
	})
}

// ticketSearch is the handler for "POST /admin/ticket". The hash param will be
// used to retrieve a ticket from the database.
func ticketSearch(c *gin.Context) {
	hash := c.PostForm("hash")

	ticket, found, err := db.GetTicketByHash(hash)
	if err != nil {
		log.Errorf("db.GetTicketByHash error (ticketHash=%s): %v", hash, err)
		c.String(http.StatusInternalServerError, "Error getting ticket from db")
		return
	}

	voteChanges, err := db.GetVoteChanges(hash)
	if err != nil {
		log.Errorf("db.GetVoteChanges error (ticketHash=%s): %v", hash, err)
		c.String(http.StatusInternalServerError, "Error getting vote changes from db")
		return
	}

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"SearchResult": searchResult{
			Hash:           hash,
			Found:          found,
			Ticket:         ticket,
			VoteChanges:    voteChanges,
			MaxVoteChanges: cfg.MaxVoteChangeRecords,
		},
		"WebApiCache":  getCache(),
		"WebApiCfg":    cfg,
		"WalletStatus": walletStatus(c),
	})
}

// adminLogin is the handler for "POST /admin". If a valid password is provided,
// the current session will be authenticated as an admin.
func adminLogin(c *gin.Context) {
	password := c.PostForm("password")

	if password != cfg.AdminPass {
		log.Warnf("Failed login attempt from %s", c.ClientIP())
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"WebApiCache":       getCache(),
			"WebApiCfg":         cfg,
			"IncorrectPassword": true,
		})
		return
	}

	setAdminStatus(true, c)
}

// adminLogout is the handler for "POST /admin/logout". The current session will
// have its admin authentication removed.
func adminLogout(c *gin.Context) {
	setAdminStatus(nil, c)
}

// downloadDatabaseBackup is the handler for "GET /backup". A binary
// representation of the whole database is generated and returned to the client.
func downloadDatabaseBackup(c *gin.Context) {
	err := db.BackupDB(c.Writer)
	if err != nil {
		log.Errorf("Error backing up database: %v", err)
		// Don't write any http body here because Content-Length has already
		// been set in db.BackupDB. Status is enough to indicate an error.
		c.Status(http.StatusInternalServerError)
	}
}

// setAdminStatus stores the authentication status of the current session and
// redirects the client to GET /admin.
func setAdminStatus(admin interface{}, c *gin.Context) {
	session := c.MustGet("session").(*sessions.Session)
	session.Values["admin"] = admin
	err := session.Save(c.Request, c.Writer)
	if err != nil {
		log.Errorf("Error saving session: %v", err)
		c.String(http.StatusInternalServerError, "Error saving session")
		return
	}

	c.Redirect(http.StatusFound, "/admin")
	c.Abort()
}
