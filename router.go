package main

import (
	"github.com/gin-gonic/gin"
)

func newRouter() *gin.Engine {
	router := gin.Default()

	api := router.Group("/api")

	api.Use()
	{
		router.GET("/fee", fee)
		router.GET("/pubkey", pubKey)
		router.GET("/payfee", payFee)
	}

	return router
}
