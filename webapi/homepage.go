// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"

	"github.com/gin-gonic/gin"
)

// vspStats is used to cache values which are commonly used by the API, so
// repeated web requests don't repeatedly trigger DB or RPC calls.
type vspStats struct {
	PubKey            string
	Voting            int64
	Voted             int64
	Revoked           int64
	VSPFee            float64
	Network           string
	UpdateTime        string
	SupportEmail      string
	VspClosed         bool
	Debug             bool
	Designation       string
	BlockHeight       uint32
	NetworkProportion float32
	RevokedProportion float32
}

var statsMtx sync.RWMutex
var stats *vspStats

func getVSPStats() *vspStats {
	statsMtx.RLock()
	defer statsMtx.RUnlock()

	return stats
}

// initVSPStats creates the struct which holds the cached VSP stats, and
// initializes it with static values.
func initVSPStats() {

	statsMtx.Lock()
	defer statsMtx.Unlock()

	stats = &vspStats{
		PubKey:       base64.StdEncoding.EncodeToString(signPubKey),
		VSPFee:       cfg.VSPFee,
		Network:      cfg.NetParams.Name,
		SupportEmail: cfg.SupportEmail,
		VspClosed:    cfg.VspClosed,
		Debug:        cfg.Debug,
		Designation:  cfg.Designation,
	}
}

// updateVSPStats updates the dynamic values in the cached VSP stats (ticket
// counts and best block height).
func updateVSPStats(ctx context.Context, db *database.VspDatabase,
	dcrd rpc.DcrdConnect, netParams *chaincfg.Params) error {

	// Update counts of voting, voted and revoked tickets.
	voting, voted, revoked, err := db.CountTickets()
	if err != nil {
		return err
	}

	// Update best block height.
	dcrdClient, err := dcrd.Client(ctx, netParams)
	if err != nil {
		return err
	}

	bestBlock, err := dcrdClient.GetBestBlockHeader()
	if err != nil {
		return err
	}

	statsMtx.Lock()
	defer statsMtx.Unlock()

	stats.UpdateTime = dateTime(time.Now().Unix())
	stats.Voting = voting
	stats.Voted = voted
	stats.Revoked = revoked
	stats.BlockHeight = bestBlock.Height
	stats.NetworkProportion = float32(voting) / float32(bestBlock.PoolSize)
	stats.RevokedProportion = float32(revoked) / float32(voted)

	return nil
}

func homepage(c *gin.Context) {
	c.HTML(http.StatusOK, "homepage.html", gin.H{
		"VspStats": getVSPStats(),
	})
}
