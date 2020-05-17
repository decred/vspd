package webapi

import (
	"context"
	"crypto/ed25519"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/gin-gonic/gin"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jrick/wsrpc/v2"
)

type Config struct {
	SignKey   ed25519.PrivateKey
	PubKey    ed25519.PublicKey
	VSPFee    float64
	NetParams *chaincfg.Params
}

var cfg Config
var db *database.VspDatabase
var nodeConnection *wsrpc.Client

func Start(ctx context.Context, requestShutdownChan chan struct{}, shutdownWg *sync.WaitGroup,
	listen string, db *database.VspDatabase, nodeConnection *wsrpc.Client, releaseMode bool, config Config) error {

	// Create TCP listener.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", listen)
	if err != nil {
		return err
	}
	log.Infof("Listening on %s", listen)

	srv := http.Server{
		Handler:      router(releaseMode),
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

	return nil
}

func router(releaseMode bool) *gin.Engine {
	// With release mode enabled, gin will only read template files once and cache them.
	// With release mode disabled, templates will be reloaded on the fly.
	if releaseMode {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.LoadHTMLGlob("templates/*")

	// Recovery middleware handles any go panics generated while processing web
	// requests. Ensures a 500 response is sent to the client rather than
	// sending no response at all.
	router.Use(gin.Recovery())

	if !releaseMode {
		// Logger middleware outputs very detailed logging of webserver requests
		// to the terminal. Does not get logged to file.
		router.Use(gin.Logger())
	}

	// Serve static web resources
	router.Static("/public", "./public/")

	router.GET("/", homepage)

	api := router.Group("/api")
	{
		api.GET("/fee", fee)
		api.POST("/feeaddress", feeAddress)
		api.GET("/pubkey", pubKey)
		api.POST("/payfee", payFee)
		api.POST("/setvotebits", setVoteBits)
		api.POST("/ticketstatus", ticketStatus)
	}

	return router
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"Message": "Welcome to dcrvsp!",
	})
}
