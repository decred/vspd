// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/dustin/go-humanize"
)

// apiCache is used to cache values which are commonly used by the API, so
// repeated web requests don't repeatedly trigger DB or RPC calls.
type apiCache struct {
	UpdateTime          string
	PubKey              string
	DatabaseSize        string
	Voting              int64
	Voted               int64
	Revoked             int64
	VotingWalletsOnline int64
	TotalVotingWallets  int64
	BlockHeight         uint32
	NetworkProportion   float32
	RevokedProportion   float32
}

var cacheMtx sync.RWMutex
var cache apiCache

func getCache() apiCache {
	cacheMtx.RLock()
	defer cacheMtx.RUnlock()

	return cache
}

// initCache creates the struct which holds the cached VSP stats, and
// initializes it with static values.
func initCache(signPubKey string) {
	cacheMtx.Lock()
	defer cacheMtx.Unlock()

	cache = apiCache{
		PubKey: signPubKey,
	}
}

// updateCache updates the dynamic values in the cache (ticket counts and best
// block height).
func updateCache(ctx context.Context, db *database.VspDatabase,
	dcrd rpc.DcrdConnect, netParams *chaincfg.Params, wallets rpc.WalletConnect) error {

	dbSize, err := db.Size()
	if err != nil {
		return err
	}

	// Get latest counts of voting, voted and revoked tickets.
	voting, voted, revoked, err := db.CountTickets()
	if err != nil {
		return err
	}

	// Get latest best block height.
	dcrdClient, _, err := dcrd.Client(ctx, netParams)
	if err != nil {
		return err
	}

	bestBlock, err := dcrdClient.GetBestBlockHeader()
	if err != nil {
		return err
	}

	if bestBlock.PoolSize == 0 {
		return errors.New("dcr node reports a network ticket pool size of zero")
	}

	clients, failedConnections := wallets.Clients(ctx, netParams)
	if len(clients) == 0 {
		log.Error("Could not connect to any wallets")
	} else if len(failedConnections) > 0 {
		log.Errorf("Failed to connect to %d wallet(s), proceeding with only %d",
			len(failedConnections), len(clients))
	}

	cacheMtx.Lock()
	defer cacheMtx.Unlock()

	cache.UpdateTime = dateTime(time.Now().Unix())
	cache.DatabaseSize = humanize.Bytes(dbSize)
	cache.Voting = voting
	cache.Voted = voted
	cache.TotalVotingWallets = int64(len(clients) + len(failedConnections))
	cache.VotingWalletsOnline = int64(len(clients))
	cache.Revoked = revoked
	cache.BlockHeight = bestBlock.Height
	cache.NetworkProportion = float32(voting) / float32(bestBlock.PoolSize)

	// Prevent dividing by zero when pool has no voted tickets.
	switch voted {
	case 0:
		if revoked == 0 {
			cache.RevokedProportion = 0
		} else {
			cache.RevokedProportion = 1
		}
	default:
		cache.RevokedProportion = float32(revoked) / float32(voted)
	}

	return nil
}
