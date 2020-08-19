// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/decred/vspd/database"

	"github.com/gin-gonic/gin"
)

type vspStats struct {
	PubKey       string
	Voting       int64
	Voted        int64
	Revoked      int64
	VSPFee       float64
	Network      string
	UpdateTime   string
	SupportEmail string
	VspClosed    bool
	Debug        bool
	Designation  string
}

var statsMtx sync.RWMutex
var stats *vspStats

func getVSPStats() *vspStats {
	statsMtx.RLock()
	defer statsMtx.RUnlock()

	return stats
}

func updateVSPStats(db *database.VspDatabase, cfg Config) error {
	voting, voted, revoked, err := db.CountTickets()
	if err != nil {
		return err
	}

	statsMtx.Lock()
	defer statsMtx.Unlock()

	stats = &vspStats{
		PubKey:       base64.StdEncoding.EncodeToString(signPubKey),
		Voting:       voting,
		Voted:        voted,
		Revoked:      revoked,
		VSPFee:       cfg.VSPFee,
		Network:      cfg.NetParams.Name,
		UpdateTime:   time.Now().Format("Mon Jan _2 15:04:05 2006"),
		SupportEmail: cfg.SupportEmail,
		VspClosed:    cfg.VspClosed,
		Debug:        cfg.Debug,
		Designation:  cfg.Designation,
	}

	return nil
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"VspStats": getVSPStats(),
	})
}
