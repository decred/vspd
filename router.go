package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func newRouter(releaseMode bool) *gin.Engine {
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
		api.POST("/ticketstatus", ticketStatus)
	}

	return router
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"Message": "Welcome to dcrvsp!",
	})
}
