package webapi

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/gin-gonic/gin"
)

type Config struct {
	SignKey              ed25519.PrivateKey
	PubKey               ed25519.PublicKey
	VSPFee               float64
	NetParams            *chaincfg.Params
	FeeAccountName       string
	FeeAddressExpiration time.Duration
}

var cfg Config
var db *database.VspDatabase
var feeWalletConnect rpc.Connect
var votingWalletConnect rpc.Connect

func Start(ctx context.Context, requestShutdownChan chan struct{}, shutdownWg *sync.WaitGroup,
	listen string, vdb *database.VspDatabase, fWalletConnect rpc.Connect, vWalletConnect rpc.Connect, debugMode bool, config Config) error {

	// Create TCP listener.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", listen)
	if err != nil {
		return err
	}
	log.Infof("Listening on %s", listen)

	srv := http.Server{
		Handler:      router(debugMode),
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
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		err = srv.Serve(listener)
		// If the server dies for any reason other than ErrServerClosed (from
		// graceful server.Shutdown), log the error and request dcrvsp be
		// shutdown.
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Unexpected webserver error: %v", err)
			requestShutdownChan <- struct{}{}
		}
	}()

	cfg = config
	db = vdb
	feeWalletConnect = fWalletConnect
	votingWalletConnect = vWalletConnect

	return nil
}

func router(debugMode bool) *gin.Engine {
	// With release mode enabled, gin will only read template files once and cache them.
	// With release mode disabled, templates will be reloaded on the fly.
	if !debugMode {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.LoadHTMLGlob("webapi/templates/*")

	// Recovery middleware handles any go panics generated while processing web
	// requests. Ensures a 500 response is sent to the client rather than
	// sending no response at all.
	router.Use(gin.Recovery())

	if debugMode {
		// Logger middleware outputs very detailed logging of webserver requests
		// to the terminal. Does not get logged to file.
		router.Use(gin.Logger())
	}

	// Serve static web resources
	router.Static("/public", "webapi/public/")

	router.GET("/", homepage)

	api := router.Group("/api")
	{
		api.GET("/fee", fee)
		api.POST("/feeaddress", feeAddress)
		api.GET("/pubkey", pubKey)
		api.POST("/payfee", payFee)
		api.POST("/setvotechoices", setVoteChoices)
		api.GET("/ticketstatus", ticketStatus)
	}

	return router
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"Message": "Welcome to dcrvsp!",
	})
}

func sendJSONResponse(resp interface{}, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
		sendErrorResponse("failed to marshal json", http.StatusInternalServerError, c)
		return
	}

	sig := ed25519.Sign(cfg.SignKey, dec)
	c.Writer.Header().Set("VSP-Signature", hex.EncodeToString(sig))

	c.JSON(http.StatusOK, resp)
}

func sendErrorResponse(errMsg string, code int, c *gin.Context) {
	c.JSON(code, gin.H{"error": errMsg})
}
