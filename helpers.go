package main

import (
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
)

func currentVoteVersion(params *chaincfg.Params) uint32 {
	var latestVersion uint32
	for version := range params.Deployments {
		if latestVersion < version {
			latestVersion = version
		}
	}
	return latestVersion
}

// isValidVoteBits returns an error if voteBits are not valid for agendas
func isValidVoteBits(params *chaincfg.Params, voteVersion uint32, voteBits uint16) bool {
	if !dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid) {
		return false
	}
	voteBits &= ^uint16(dcrutil.BlockValid)

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
