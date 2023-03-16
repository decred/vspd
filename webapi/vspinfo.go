// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	"github.com/decred/vspd/types/v2"
	"github.com/decred/vspd/version"
	"github.com/gin-gonic/gin"
)

// vspInfo is the handler for "GET /api/v3/vspinfo".
func (s *Server) vspInfo(c *gin.Context) {
	cachedStats := s.cache.getData()
	s.sendJSONResponse(types.VspInfoResponse{
		APIVersions:         []int64{3},
		Timestamp:           time.Now().Unix(),
		PubKey:              s.signPubKey,
		FeePercentage:       s.cfg.VSPFee,
		Network:             s.cfg.NetParams.Name,
		VspClosed:           s.cfg.VspClosed,
		VspClosedMsg:        s.cfg.VspClosedMsg,
		VspdVersion:         version.String(),
		Voting:              cachedStats.Voting,
		Voted:               cachedStats.Voted,
		TotalVotingWallets:  cachedStats.TotalVotingWallets,
		VotingWalletsOnline: cachedStats.VotingWalletsOnline,
		Revoked:             cachedStats.Revoked,
		BlockHeight:         cachedStats.BlockHeight,
		NetworkProportion:   cachedStats.NetworkProportion,
	}, c)
}
