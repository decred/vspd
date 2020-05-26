package webapi

import (
	"errors"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
)

type addressGenerator struct {
	external      *hdkeychain.ExtendedKey
	netParams     *chaincfg.Params
	lastUsedIndex uint32
}

func newAddressGenerator(xPub string, netParams *chaincfg.Params, lastUsedIdx uint32) (*addressGenerator, error) {
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
	}, nil
}

// NextAddress increments the last used address counter and returns a new
// address. It will skip any address index which causes an ErrInvalidChild.
// Not safe for concurrent access.
func (m *addressGenerator) NextAddress() (string, uint32, error) {
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
			if err == hdkeychain.ErrInvalidChild {
				invalidChildren++
				log.Warnf("Generating address for index %d failed: %v", m.lastUsedIndex, err)
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
	pkHash := dcrutil.Hash160(key.SerializedPubKey())
	addr, err := dcrutil.NewAddressPubKeyHash(pkHash, m.netParams, dcrec.STEcdsaSecp256k1)
	if err != nil {
		return "", 0, err
	}

	return addr.String(), m.lastUsedIndex, nil
}
