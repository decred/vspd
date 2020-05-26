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

var homepageData *gin.H

var cfg Config
var db *database.VspDatabase
var feeWalletConnect rpc.Connect
var votingWalletConnect rpc.Connect
var addrGen *addressGenerator

func Start(ctx context.Context, requestShutdownChan chan struct{}, shutdownWg *sync.WaitGroup,
	listen string, vdb *database.VspDatabase, fWalletConnect rpc.Connect, vWalletConnect rpc.Connect, debugMode bool, feeXPub string, config Config) error {

	// Populate template data before starting webserver.
	var err error
	homepageData, err = updateHomepageData(vdb, config)
	if err != nil {
		return fmt.Errorf("could not initialize homepage data: %v", err)
	}

	// Get the last used address index from the database, and use it to
	// initialize the address generator.
	idx, err := vdb.GetLastAddressIndex()
	if err != nil {
		return fmt.Errorf("GetLastAddressIndex error: %v", err)
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
		// graceful server.Shutdown), log the error and request dcrvsp be
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
				homepageData, err = updateHomepageData(db, cfg)
				if err != nil {
					log.Errorf("Failed to update homepage data: %v", err)
				}
			}
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

	// These routes have no extra middleware. They can be accessed by anybody.
	router.GET("/", homepage)
	router.GET("/api/fee", fee)
	router.GET("/api/pubkey", pubKey)

	// These API routes access the fee wallet and they need authentication.
	feeOnly := router.Group("/api").Use(
		withFeeWalletClient(), vspAuth(),
	)
	feeOnly.POST("/feeaddress", feeAddress)
	feeOnly.GET("/ticketstatus", ticketStatus)

	// These API routes access the fee wallet and the voting wallets, and they
	// need authentication.
	both := router.Group("/api").Use(
		withFeeWalletClient(), withVotingWalletClient(), vspAuth(),
	)
	both.POST("/payfee", payFee)
	both.POST("/setvotechoices", setVoteChoices)

	return router
}

func updateHomepageData(db *database.VspDatabase, cfg Config) (*gin.H, error) {
	total, feePaid, err := db.CountTickets()
	if err != nil {
		return nil, err
	}
	return &gin.H{
		"Message":        "Welcome to dcrvsp!",
		"TotalTickets":   total,
		"FeePaidTickets": feePaid,
		"VSPFee":         cfg.VSPFee,
		"Network":        cfg.NetParams.Name,
		"UpdateTime":     time.Now().Format("Mon Jan _2 15:04:05 2006"),
	}, nil
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", homepageData)
}

func sendJSONResponse(resp interface{}, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
		sendErrorResponse("failed to marshal json", http.StatusInternalServerError, c)
		return
	}

	sig := ed25519.Sign(cfg.SignKey, dec)
	c.Writer.Header().Set("VSP-Server-Signature", hex.EncodeToString(sig))

	c.AbortWithStatusJSON(http.StatusOK, resp)
}

func sendErrorResponse(errMsg string, code int, c *gin.Context) {
	c.AbortWithStatusJSON(code, gin.H{"error": errMsg})
}
