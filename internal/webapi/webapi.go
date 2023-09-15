// Copyright (c) 2020-2023 The Decred developers
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

	"github.com/decred/slog"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/internal/config"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/types/v2"
	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

type Config struct {
	Listen               string
	VSPFee               float64
	Network              *config.Network
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

type WebAPI struct {
	cfg         Config
	db          *database.VspDatabase
	log         slog.Logger
	addrGen     *addressGenerator
	cache       *cache
	signPrivKey ed25519.PrivateKey
	signPubKey  ed25519.PublicKey
}

func Start(ctx context.Context, requestShutdown func(), shutdownWg *sync.WaitGroup,
	vdb *database.VspDatabase, log slog.Logger, dcrd rpc.DcrdConnect,
	wallets rpc.WalletConnect, cfg Config) error {

	// Get keys for signing API responses from the database.
	signPrivKey, signPubKey, err := vdb.KeyPair()
	if err != nil {
		return fmt.Errorf("db.Keypair error: %w", err)
	}

	// Populate cached VSP stats before starting webserver.
	encodedPubKey := base64.StdEncoding.EncodeToString(signPubKey)
	cache := newCache(encodedPubKey, log, vdb, dcrd, wallets)
	err = cache.update()
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
	addrGen, err := newAddressGenerator(feeXPub, cfg.Network.Params, idx, log)
	if err != nil {
		return fmt.Errorf("failed to initialize fee address generator: %w", err)
	}

	// Get the secret key used to initialize the cookie store.
	cookieSecret, err := vdb.CookieSecret()
	if err != nil {
		return fmt.Errorf("db.GetCookieSecret error: %w", err)
	}

	w := &WebAPI{
		cfg:         cfg,
		db:          vdb,
		log:         log,
		addrGen:     addrGen,
		cache:       cache,
		signPrivKey: signPrivKey,
		signPubKey:  signPubKey,
	}

	// Create TCP listener.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", cfg.Listen)
	if err != nil {
		return err
	}
	log.Infof("Listening on %s", cfg.Listen)

	srv := http.Server{
		Handler:      w.router(cookieSecret, dcrd, wallets),
		ReadTimeout:  5 * time.Second,  // slow requests should not hold connections opened
		WriteTimeout: 60 * time.Second, // hung responses must die
	}

	// Add the graceful shutdown to the waitgroup.
	shutdownWg.Add(1)
	go func() {
		// Wait until shutdown is signaled before shutting down.
		<-ctx.Done()

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
	if w.cfg.Debug {
		refresh = 1 * time.Second
	} else {
		refresh = 1 * time.Minute
	}
	shutdownWg.Add(1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				shutdownWg.Done()
				return
			case <-time.After(refresh):
				err := w.cache.update()
				if err != nil {
					log.Errorf("Failed to update cached VSP stats: %v", err)
				}
			}
		}
	}()

	return nil
}

func (w *WebAPI) router(cookieSecret []byte, dcrd rpc.DcrdConnect, wallets rpc.WalletConnect) *gin.Engine {
	// With release mode enabled, gin will only read template files once and cache them.
	// With release mode disabled, templates will be reloaded on the fly.
	if !w.cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	explorerURL := w.cfg.Network.BlockExplorerURL

	// Add custom functions for use in templates.
	router.SetFuncMap(template.FuncMap{
		"txURL":            txURL(explorerURL),
		"addressURL":       addressURL(explorerURL),
		"blockURL":         blockURL(explorerURL),
		"dateTime":         dateTime,
		"stripWss":         stripWss,
		"indentJSON":       indentJSON(w.log),
		"atomsToDCR":       atomsToDCR,
		"float32ToPercent": float32ToPercent,
		"comma":            humanize.Comma,
	})

	router.LoadHTMLGlob("internal/webapi/templates/*.html")

	// Recovery middleware handles any go panics generated while processing web
	// requests. Ensures a 500 response is sent to the client rather than
	// sending no response at all.
	router.Use(recovery(w.log))

	if w.cfg.Debug {
		// Logger middleware outputs very detailed logging of webserver requests
		// to the terminal. Does not get logged to file.
		router.Use(gin.Logger())
	}

	// Serve static web resources
	router.Static("/public", "internal/webapi/public/")

	// Create a cookie store for persisting admin session information.
	cookieStore := sessions.NewCookieStore(cookieSecret)

	// API routes.

	api := router.Group("/api/v3")
	api.GET("/vspinfo", w.vspInfo)
	api.POST("/setaltsignaddr", w.vspMustBeOpen, w.withDcrdClient(dcrd), w.broadcastTicket, w.vspAuth, w.setAltSignAddr)
	api.POST("/feeaddress", w.vspMustBeOpen, w.withDcrdClient(dcrd), w.broadcastTicket, w.vspAuth, w.feeAddress)
	api.POST("/ticketstatus", w.withDcrdClient(dcrd), w.vspAuth, w.ticketStatus)
	api.POST("/payfee", w.vspMustBeOpen, w.withDcrdClient(dcrd), w.vspAuth, w.payFee)
	api.POST("/setvotechoices", w.withDcrdClient(dcrd), w.withWalletClients(wallets), w.vspAuth, w.setVoteChoices)

	// Website routes.

	router.GET("", w.homepage)

	login := router.Group("/admin").Use(
		w.withSession(cookieStore),
	)

	// Limit login attempts to 3 per second.
	loginRateLmiter := rateLimit(3, func(c *gin.Context) {
		w.log.Warnf("Login rate limit exceeded by %s", c.ClientIP())
		c.HTML(http.StatusTooManyRequests, "login.html", gin.H{
			"WebApiCache":    w.cache.getData(),
			"WebApiCfg":      w.cfg,
			"FailedLoginMsg": "Rate limit exceeded",
		})
	})
	login.POST("", loginRateLmiter, w.adminLogin)

	admin := router.Group("/admin").Use(
		w.withWalletClients(wallets), w.withSession(cookieStore), w.requireAdmin,
	)
	admin.GET("", w.withDcrdClient(dcrd), w.adminPage)
	admin.POST("/ticket", w.withDcrdClient(dcrd), w.ticketSearch)
	admin.GET("/backup", w.downloadDatabaseBackup)
	admin.POST("/logout", w.adminLogout)

	// Require Basic HTTP Auth on /admin/status endpoint.
	basic := router.Group("/admin").Use(
		w.withDcrdClient(dcrd), w.withWalletClients(wallets), gin.BasicAuth(gin.Accounts{
			"admin": w.cfg.AdminPass,
		}),
	)
	basic.GET("/status", w.statusJSON)

	return router
}

// sendJSONResponse serializes the provided response, signs it, and sends the
// response to the client with a 200 OK status. Returns the seralized response
// and the signature.
func (w *WebAPI) sendJSONResponse(resp any, c *gin.Context) (string, string) {
	dec, err := json.Marshal(resp)
	if err != nil {
		w.log.Errorf("JSON marshal error: %v", err)
		w.sendError(types.ErrInternalError, c)
		return "", ""
	}

	sig := ed25519.Sign(w.signPrivKey, dec)
	sigStr := base64.StdEncoding.EncodeToString(sig)
	c.Writer.Header().Set("VSP-Server-Signature", sigStr)

	c.AbortWithStatusJSON(http.StatusOK, resp)

	return string(dec), sigStr
}

// sendError sends an error response with the provided error code and the
// default message for that code.
func (w *WebAPI) sendError(e types.ErrorCode, c *gin.Context) {
	msg := e.DefaultMessage()
	w.sendErrorWithMsg(msg, e, c)
}

// sendErrorWithMsg sends an error response with the provided error code and
// message.
func (w *WebAPI) sendErrorWithMsg(msg string, e types.ErrorCode, c *gin.Context) {
	status := e.HTTPStatus()

	resp := types.ErrorResponse{
		Code:    e,
		Message: msg,
	}

	// Try to sign the error response. If it fails, send it without a signature.
	dec, err := json.Marshal(resp)
	if err != nil {
		w.log.Warnf("Sending error response without signature: %v", err)
	} else {
		sig := ed25519.Sign(w.signPrivKey, dec)
		c.Writer.Header().Set("VSP-Server-Signature", base64.StdEncoding.EncodeToString(sig))
	}

	c.AbortWithStatusJSON(status, resp)
}
