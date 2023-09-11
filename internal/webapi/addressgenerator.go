// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"errors"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/slog"
)

type addressGenerator struct {
	external      *hdkeychain.ExtendedKey
	netParams     *chaincfg.Params
	lastUsedIndex uint32
	log           slog.Logger
}

func newAddressGenerator(xPub string, netParams *chaincfg.Params, lastUsedIdx uint32, log slog.Logger) (*addressGenerator, error) {
	xPubKey, err := hdkeychain.NewKeyFromString(xPub, netParams)
	if err != nil {
		return nil, err
	}

	if xPubKey.IsPrivate() {
		return nil, errors.New("not a public key")
	}

	// Derive the extended key for the external chain.
	external, err := xPubKey.Child(0)
	if err != nil {
		return nil, err
	}

	return &addressGenerator{
		external:      external,
		netParams:     netParams,
		lastUsedIndex: lastUsedIdx,
		log:           log,
	}, nil
}

// nextAddress increments the last used address counter and returns a new
// address. It will skip any address index which causes an ErrInvalidChild.
// Not safe for concurrent access.
func (m *addressGenerator) nextAddress() (string, uint32, error) {
	var key *hdkeychain.ExtendedKey
	var err error

	// There is a small chance that generating addresses for a given index can
	// fail with ErrInvalidChild, so loop until we find an index which works.
	// See the hdkeychain.ExtendedKey.Child docs for more info.
	invalidChildren := 0
	for {
		m.lastUsedIndex++
		key, err = m.external.Child(m.lastUsedIndex)
		if err != nil {
			if errors.Is(err, hdkeychain.ErrInvalidChild) {
				invalidChildren++
				m.log.Warnf("Generating address for index %d failed: %v", m.lastUsedIndex, err)
				// If this happens 3 times, something is seriously wrong, so
				// return an error.
				if invalidChildren > 2 {
					return "", 0, errors.New("multiple invalid children generated for key")
				}
				continue
			}
			return "", 0, err
		}
		break
	}

	// Convert to a standard pay-to-pubkey-hash address.
	pkHash := stdaddr.Hash160(key.SerializedPubKey())
	addr, err := stdaddr.NewAddressPubKeyHashEcdsaSecp256k1V0(pkHash, m.netParams)
	if err != nil {
		return "", 0, err
	}

	return addr.String(), m.lastUsedIndex, nil
}
