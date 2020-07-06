package webapi

import (
	"net/http"

	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// WalletStatus describes the current status of a single voting wallet.
type WalletStatus struct {
	Connected       bool
	InfoError       bool
	DaemonConnected bool
	VoteVersion     uint32
	Unlocked        bool
	Voting          bool
	BestBlockError  bool
	BestBlockHeight int64
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

// adminPage is the handler for "GET /admin".
func adminPage(c *gin.Context) {
	c.HTML(http.StatusOK, "admin.html", gin.H{
		"VspStats":     getVSPStats(),
		"WalletStatus": walletStatus(c),
	})
}

// ticketSearch is the handler for "POST /admin/ticket". The hash param will be
// used to retrieve a ticket from the database.
func ticketSearch(c *gin.Context) {
	hash := c.PostForm("hash")

	ticket, found, err := db.GetTicketByHash(hash)
	if err != nil {
		log.Errorf("db.GetTicketByHash error: %v", err)
		c.String(http.StatusInternalServerError, "Error getting ticket from db")
		return
	}

	c.HTML(http.StatusOK, "admin.html", gin.H{
		"SearchResult": gin.H{
			"Hash":   hash,
			"Found":  found,
			"Ticket": ticket,
		},
		"VspStats":     getVSPStats(),
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
			"VspStats":          getVSPStats(),
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
