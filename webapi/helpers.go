package webapi

import (
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
)

// isValidVoteBits checks if voteBits are valid for the most recent agendas.
func isValidVoteBits(params *chaincfg.Params, voteBits uint16) bool {

	if !dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid) {
		return false
	}
	voteBits &= ^uint16(dcrutil.BlockValid)

	// Get the most recent vote version.
	var voteVersion uint32
	for version := range params.Deployments {
		if voteVersion < version {
			voteVersion = version
		}
	}

	var availVoteBits uint16
	for _, vote := range params.Deployments[voteVersion] {
		availVoteBits |= vote.Vote.Mask

		isValid := false
		maskedBits := voteBits & vote.Vote.Mask
		for _, c := range vote.Vote.Choices {
			if c.Bits == maskedBits {
				isValid = true
				break
			}
		}
		if !isValid {
			return false
		}
	}
	return true
}
