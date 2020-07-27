package webapi

import (
	"time"

	"github.com/decred/vspd/version"
	"github.com/gin-gonic/gin"
)

// vspInfo is the handler for "GET /api/v3/vspinfo".
func vspInfo(c *gin.Context) {
	cachedStats := getVSPStats()
	sendJSONResponse(vspInfoResponse{
		APIVersions:   []int64{3},
		Timestamp:     time.Now().Unix(),
		PubKey:        signPubKey,
		FeePercentage: cfg.VSPFee,
		Network:       cfg.NetParams.Name,
		VspClosed:     cfg.VspClosed,
		VspdVersion:   version.String(),
		Voting:        cachedStats.Voting,
		Voted:         cachedStats.Voted,
		Revoked:       cachedStats.Revoked,
	}, c)
}
