package webapi

import (
	"time"

	"github.com/gin-gonic/gin"
)

// vspStatus is the handler for "GET /vspstatus".
func vspStatus(c *gin.Context) {
	sendJSONResponse(vspStatusResponse{
		Timestamp:     time.Now().Unix(),
		PubKey:        signPubKey,
		FeePercentage: cfg.VSPFee,
		Network:       cfg.NetParams.Name,
		VspClosed:     cfg.VspClosed,
	}, c)
}
