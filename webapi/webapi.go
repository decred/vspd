package webapi

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/gin-gonic/gin"
)

type Config struct {
	VSPFee               float64
	NetParams            *chaincfg.Params
	FeeAccountName       string
	FeeAddressExpiration time.Duration
	SupportEmail         string
	VspClosed            bool
}

const (
	// TODO: Make this configurable or get it from RPC.
	relayFee = 0.0001
)

var cfg Config
var db *database.VspDatabase
var dcrd rpc.DcrdConnect
var wallets rpc.WalletConnect
var addrGen *addressGenerator
var signPrivKey ed25519.PrivateKey
var signPubKey ed25519.PublicKey

func Start(ctx context.Context, requestShutdownChan chan struct{}, shutdownWg *sync.WaitGroup,
	listen string, vdb *database.VspDatabase, dConnect rpc.DcrdConnect, wConnect rpc.WalletConnect, debugMode bool, config Config) error {

	cfg = config
	db = vdb
	dcrd = dConnect
	wallets = wConnect

	var err error

	// Get keys for signing API responses from the database.
	signPrivKey, signPubKey, err = vdb.KeyPair()
	if err != nil {
		return fmt.Errorf("Failed to get keypair: %v", err)
	}

	// Populate template data before starting webserver.
	stats, err = updateVSPStats(vdb, config)
	if err != nil {
		return fmt.Errorf("could not initialize homepage data: %v", err)
	}

	// Get the last used address index and the feeXpub from the database, and
	// use them to initialize the address generator.
	idx, err := vdb.GetLastAddressIndex()
	if err != nil {
		return fmt.Errorf("GetLastAddressIndex error: %v", err)
	}
	feeXPub, err := vdb.GetFeeXPub()
	if err != nil {
		return fmt.Errorf("GetFeeXPub error: %v", err)
	}
	addrGen, err = newAddressGenerator(feeXPub, config.NetParams, idx)
	if err != nil {
		return fmt.Errorf("failed to initialize fee address generator: %v", err)
	}

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
		// graceful server.Shutdown), log the error and request vspd be
		// shutdown.
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Unexpected webserver error: %v", err)
			requestShutdownChan <- struct{}{}
		}
	}()

	// Use a ticker to update template data.
	var refresh time.Duration
	if debugMode {
		refresh = 1 * time.Second
	} else {
		refresh = 5 * time.Minute
	}
	shutdownWg.Add(1)
	go func() {
		ticker := time.NewTicker(refresh)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				shutdownWg.Done()
				return
			case <-ticker.C:
				stats, err = updateVSPStats(db, cfg)
				if err != nil {
					log.Errorf("Failed to update homepage data: %v", err)
				}
			}
		}
	}()

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

	// These routes have no extra middleware. They can be accessed by anybody.
	router.GET("/", homepage)
	router.GET("/api/vspinfo", vspInfo)

	// These API routes access dcrd and they need authentication.
	feeOnly := router.Group("/api").Use(
		withDcrdClient(), vspAuth(),
	)
	feeOnly.POST("/feeaddress", feeAddress)
	feeOnly.GET("/ticketstatus", ticketStatus)
	feeOnly.POST("/payfee", payFee)

	// These API routes access dcrd and the voting wallets, and they need
	// authentication.
	both := router.Group("/api").Use(
		withDcrdClient(), withWalletClients(), vspAuth(),
	)
	both.POST("/setvotechoices", setVoteChoices)

	return router
}

func sendJSONResponse(resp interface{}, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
		sendErrorResponse("failed to marshal json", http.StatusInternalServerError, c)
		return
	}

	sig := ed25519.Sign(signPrivKey, dec)
	c.Writer.Header().Set("VSP-Server-Signature", hex.EncodeToString(sig))

	c.AbortWithStatusJSON(http.StatusOK, resp)
}

func sendErrorResponse(errMsg string, code int, c *gin.Context) {
	c.AbortWithStatusJSON(code, gin.H{"error": errMsg})
}
