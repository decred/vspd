// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type Config struct {
	VSPFee               float64
	NetParams            *chaincfg.Params
	BlockExplorerURL     string
	FeeAccountName       string
	SupportEmail         string
	VspClosed            bool
	VspClosedMsg         string
	AdminPass            string
	Debug                bool
	Designation          string
	MaxVoteChangeRecords int
	VspdVersion          string
}

const (
	// requiredConfs is the number of confirmations required to consider a
	// ticket purchase or a fee transaction to be final.
	requiredConfs = 6
	// feeAddressExpiration is the length of time a fee returned by /feeaddress
	// remains valid. After this time, a new fee must be requested.
	feeAddressExpiration = 1 * time.Hour
)

// Hard-coded keys used for storing values in the web context.
const (
	sessionKey           = "Session"
	dcrdKey              = "DcrdClient"
	dcrdHostKey          = "DcrdHostname"
	dcrdErrorKey         = "DcrdClientErr"
	walletsKey           = "WalletClients"
	failedWalletsKey     = "FailedWalletClients"
	requestBytesKey      = "RequestBytes"
	ticketKey            = "Ticket"
	knownTicketKey       = "KnownTicket"
	commitmentAddressKey = "CommitmentAddress"
)

type Server struct {
	cfg         Config
	db          *database.VspDatabase
	log         slog.Logger
	addrGen     *addressGenerator
	cache       *cache
	signPrivKey ed25519.PrivateKey
	signPubKey  ed25519.PublicKey
}

func Start(shutdownCtx context.Context, requestShutdown func(), shutdownWg *sync.WaitGroup,
	listen string, vdb *database.VspDatabase, log slog.Logger, dcrd rpc.DcrdConnect,
	wallets rpc.WalletConnect, config Config) error {

	s := &Server{
		cfg: config,
		db:  vdb,
		log: log,
	}

	var err error

	// Get keys for signing API responses from the database.
	s.signPrivKey, s.signPubKey, err = vdb.KeyPair()
	if err != nil {
		return fmt.Errorf("db.Keypair error: %w", err)
	}

	// Populate cached VSP stats before starting webserver.
	s.cache = newCache(base64.StdEncoding.EncodeToString(s.signPubKey), log)
	err = s.cache.update(vdb, dcrd, wallets)
	if err != nil {
		log.Errorf("Could not initialize VSP stats cache: %v", err)
	}

	// Get the last used address index and the feeXpub from the database, and
	// use them to initialize the address generator.
	idx, err := vdb.GetLastAddressIndex()
	if err != nil {
		return fmt.Errorf("db.GetLastAddressIndex error: %w", err)
	}
	feeXPub, err := vdb.FeeXPub()
	if err != nil {
		return fmt.Errorf("db.GetFeeXPub error: %w", err)
	}
	s.addrGen, err = newAddressGenerator(feeXPub, config.NetParams, idx, log)
	if err != nil {
		return fmt.Errorf("failed to initialize fee address generator: %w", err)
	}

	// Get the secret key used to initialize the cookie store.
	cookieSecret, err := vdb.CookieSecret()
	if err != nil {
		return fmt.Errorf("db.GetCookieSecret error: %w", err)
	}

	// Create TCP listener.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(shutdownCtx, "tcp", listen)
	if err != nil {
		return err
	}
	log.Infof("Listening on %s", listen)

	srv := http.Server{
		Handler:      s.router(cookieSecret, dcrd, wallets),
		ReadTimeout:  5 * time.Second,  // slow requests should not hold connections opened
		WriteTimeout: 60 * time.Second, // hung responses must die
	}

	// Add the graceful shutdown to the waitgroup.
	shutdownWg.Add(1)
	go func() {
		// Wait until shutdown is signaled before shutting down.
		<-shutdownCtx.Done()

		log.Debug("Stopping webserver...")
		// Give the webserver 5 seconds to finish what it is doing.
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(timeoutCtx); err != nil {
			log.Errorf("Failed to stop webserver cleanly: %v", err)
		} else {
			log.Debug("Webserver stopped")
		}
		shutdownWg.Done()
	}()

	// Start webserver.
	go func() {
		err := srv.Serve(listener)
		// If the server dies for any reason other than ErrServerClosed (from
		// graceful server.Shutdown), log the error and request vspd be
		// shutdown.
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("Unexpected webserver error: %v", err)
			requestShutdown()
		}
	}()

	// Periodically update cached VSP stats.
	var refresh time.Duration
	if s.cfg.Debug {
		refresh = 1 * time.Second
	} else {
		refresh = 1 * time.Minute
	}
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-shutdownCtx.Done():
				shutdownWg.Done()
				return
			case <-time.After(refresh):
				err := s.cache.update(vdb, dcrd, wallets)
				if err != nil {
					log.Errorf("Failed to update cached VSP stats: %v", err)
				}
			}
		}
	}()

	return nil
}

func (s *Server) router(cookieSecret []byte, dcrd rpc.DcrdConnect, wallets rpc.WalletConnect) *gin.Engine {
	// With release mode enabled, gin will only read template files once and cache them.
	// With release mode disabled, templates will be reloaded on the fly.
	if !s.cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Add custom functions for use in templates.
	router.SetFuncMap(template.FuncMap{
		"txURL":            txURL(s.cfg.BlockExplorerURL),
		"addressURL":       addressURL(s.cfg.BlockExplorerURL),
		"blockURL":         blockURL(s.cfg.BlockExplorerURL),
		"dateTime":         dateTime,
		"stripWss":         stripWss,
		"indentJSON":       indentJSON(s.log),
		"atomsToDCR":       atomsToDCR,
		"float32ToPercent": float32ToPercent,
		"comma":            humanize.Comma,
	})

	router.LoadHTMLGlob("webapi/templates/*.html")

	// Recovery middleware handles any go panics generated while processing web
	// requests. Ensures a 500 response is sent to the client rather than
	// sending no response at all.
	router.Use(Recovery(s.log))

	if s.cfg.Debug {
		// Logger middleware outputs very detailed logging of webserver requests
		// to the terminal. Does not get logged to file.
		router.Use(gin.Logger())
	}

	// Serve static web resources
	router.Static("/public", "webapi/public/")

	// Create a cookie store for persisting admin session information.
	cookieStore := sessions.NewCookieStore(cookieSecret)

	// API routes.

	api := router.Group("/api/v3")
	api.GET("/vspinfo", s.vspInfo)
	api.POST("/setaltsignaddr", s.withDcrdClient(dcrd), s.broadcastTicket, s.vspAuth, s.setAltSignAddr)
	api.POST("/feeaddress", s.withDcrdClient(dcrd), s.broadcastTicket, s.vspAuth, s.feeAddress)
	api.POST("/ticketstatus", s.withDcrdClient(dcrd), s.vspAuth, s.ticketStatus)
	api.POST("/payfee", s.withDcrdClient(dcrd), s.vspAuth, s.payFee)
	api.POST("/setvotechoices", s.withDcrdClient(dcrd), s.withWalletClients(wallets), s.vspAuth, s.setVoteChoices)

	// Website routes.

	router.GET("", s.homepage)

	login := router.Group("/admin").Use(
		s.withSession(cookieStore),
	)
	login.POST("", s.adminLogin)

	admin := router.Group("/admin").Use(
		s.withWalletClients(wallets), s.withSession(cookieStore), s.requireAdmin,
	)
	admin.GET("", s.withDcrdClient(dcrd), s.adminPage)
	admin.POST("/ticket", s.withDcrdClient(dcrd), s.ticketSearch)
	admin.GET("/backup", s.downloadDatabaseBackup)
	admin.POST("/logout", s.adminLogout)

	// Require Basic HTTP Auth on /admin/status endpoint.
	basic := router.Group("/admin").Use(
		s.withDcrdClient(dcrd), s.withWalletClients(wallets), gin.BasicAuth(gin.Accounts{
			"admin": s.cfg.AdminPass,
		}),
	)
	basic.GET("/status", s.statusJSON)

	return router
}

// sendJSONResponse serializes the provided response, signs it, and sends the
// response to the client with a 200 OK status. Returns the seralized response
// and the signature.
func (s *Server) sendJSONResponse(resp interface{}, c *gin.Context) (string, string) {
	dec, err := json.Marshal(resp)
	if err != nil {
		s.log.Errorf("JSON marshal error: %v", err)
		s.sendError(ErrInternalError, c)
		return "", ""
	}

	sig := ed25519.Sign(s.signPrivKey, dec)
	sigStr := base64.StdEncoding.EncodeToString(sig)
	c.Writer.Header().Set("VSP-Server-Signature", sigStr)

	c.AbortWithStatusJSON(http.StatusOK, resp)

	return string(dec), sigStr
}

// sendError sends an error response with the provided error code and the
// default message for that code.
func (s *Server) sendError(e ErrorCode, c *gin.Context) {
	msg := e.DefaultMessage()
	s.sendErrorWithMsg(msg, e, c)
}

// sendErrorWithMsg sends an error response with the provided error code and
// message.
func (s *Server) sendErrorWithMsg(msg string, e ErrorCode, c *gin.Context) {
	status := e.HTTPStatus()

	resp := APIError{
		Code:    int64(e),
		Message: msg,
	}

	// Try to sign the error response. If it fails, send it without a signature.
	dec, err := json.Marshal(resp)
	if err != nil {
		s.log.Warnf("Sending error response without signature: %v", err)
	} else {
		sig := ed25519.Sign(s.signPrivKey, dec)
		c.Writer.Header().Set("VSP-Server-Signature", base64.StdEncoding.EncodeToString(sig))
	}

	c.AbortWithStatusJSON(status, resp)
}
