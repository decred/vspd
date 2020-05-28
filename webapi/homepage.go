package webapi

import (
	"net/http"
	"time"

	"github.com/jholdstock/vspd/database"

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
}

var stats *vspStats

func updateVSPStats(db *database.VspDatabase, cfg Config) (*vspStats, error) {
	total, feeConfirmed, err := db.CountTickets()
	if err != nil {
		return nil, err
	}
	return &vspStats{
		PubKey:              signPubKey,
		TotalTickets:        total,
		FeeConfirmedTickets: feeConfirmed,
		VSPFee:              cfg.VSPFee,
		Network:             cfg.NetParams.Name,
		UpdateTime:          time.Now().Format("Mon Jan _2 15:04:05 2006"),
		SupportEmail:        cfg.SupportEmail,
		VspClosed:           cfg.VspClosed,
	}, nil
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", stats)
}
