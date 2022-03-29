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

// cache is used to store values which are commonly used by the API, so
// repeated web requests don't repeatedly trigger DB or RPC calls.
type cache struct {
	// data is the cached data.
	data cacheData
	// mtx must be held to read/write cache data.
	mtx sync.RWMutex
}

type cacheData struct {
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

func (c *cache) getData() cacheData {
	c.mtx.RLock()
	defer c.mtx.RUnlock()

	return c.data
}

// newCache creates a new cache and initializes it with static values.
func newCache(signPubKey string) *cache {
	return &cache{
		data: cacheData{
			PubKey: signPubKey,
		},
	}
}

// update will use the provided database and RPC connections to update the
// dynamic values in the cache.
func (c *cache) update(ctx context.Context, db *database.VspDatabase,
	dcrd rpc.DcrdConnect, wallets rpc.WalletConnect, netParams *chaincfg.Params) error {

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

	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.data.UpdateTime = dateTime(time.Now().Unix())
	c.data.DatabaseSize = humanize.Bytes(dbSize)
	c.data.Voting = voting
	c.data.Voted = voted
	c.data.TotalVotingWallets = int64(len(clients) + len(failedConnections))
	c.data.VotingWalletsOnline = int64(len(clients))
	c.data.Revoked = revoked
	c.data.BlockHeight = bestBlock.Height
	c.data.NetworkProportion = float32(voting) / float32(bestBlock.PoolSize)

	// Prevent dividing by zero when pool has no voted tickets.
	switch voted {
	case 0:
		if revoked == 0 {
			c.data.RevokedProportion = 0
		} else {
			c.data.RevokedProportion = 1
		}
	default:
		c.data.RevokedProportion = float32(revoked) / float32(voted)
	}

	return nil
}
