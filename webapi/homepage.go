package webapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/decred/vspd/database"

	"github.com/gin-gonic/gin"
)

type vspStats struct {
	PubKey              []byte
	TotalTickets        int
	FeeConfirmedTickets int
	VSPFee              float64
	Network             string
	UpdateTime          string
	SupportEmail        string
	VspClosed           bool
	Debug               bool
}

var statsMtx sync.RWMutex
var stats *vspStats

func getVSPStats() *vspStats {
	statsMtx.RLock()
	defer statsMtx.RUnlock()

	return stats
}

func updateVSPStats(db *database.VspDatabase, cfg Config) error {
	total, feeConfirmed, err := db.CountTickets()
	if err != nil {
		return err
	}

	statsMtx.Lock()
	defer statsMtx.Unlock()

	stats = &vspStats{
		PubKey:              signPubKey,
		TotalTickets:        total,
		FeeConfirmedTickets: feeConfirmed,
		VSPFee:              cfg.VSPFee,
		Network:             cfg.NetParams.Name,
		UpdateTime:          time.Now().Format("Mon Jan _2 15:04:05 2006"),
		SupportEmail:        cfg.SupportEmail,
		VspClosed:           cfg.VspClosed,
		Debug:               cfg.Debug,
	}

	return nil
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"VspStats": getVSPStats(),
	})
}
