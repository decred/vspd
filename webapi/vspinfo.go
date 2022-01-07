// Copyright (c) 2020-2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	"github.com/decred/vspd/version"
	"github.com/gin-gonic/gin"
)

// vspInfo is the handler for "GET /api/v3/vspinfo".
func vspInfo(c *gin.Context) {
	cachedStats := getCache()
	resp, respSig := prepareJSONResponse(vspInfoResponse{
		APIVersions:       []int64{3},
		Timestamp:         time.Now().Unix(),
		PubKey:            signPubKey,
		FeePercentage:     cfg.VSPFee,
		Network:           cfg.NetParams.Name,
		VspClosed:         cfg.VspClosed,
		VspClosedMsg:      cfg.VspClosedMsg,
		VspdVersion:       version.String(),
		Voting:            cachedStats.Voting,
		Voted:             cachedStats.Voted,
		Revoked:           cachedStats.Revoked,
		BlockHeight:       cachedStats.BlockHeight,
		NetworkProportion: cachedStats.NetworkProportion,
	}, c)
	sendJSONSuccess(resp, respSig, c)
}
