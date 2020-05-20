package webapi

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/dcrutil/v3"
)

func TestVoteBits(t *testing.T) {
	var tests = []struct {
		voteBits uint16
		isValid  bool
	}{
		{0, false},
		{dcrutil.BlockValid, true},
		{dcrutil.BlockValid | 0x0002, true},
		{dcrutil.BlockValid | 0x0003, true},
		{dcrutil.BlockValid | 0x0004, true},
		{dcrutil.BlockValid | 0x0005, true},
		{dcrutil.BlockValid | 0x0006, false},
		{dcrutil.BlockValid | 0x0007, false},
		{dcrutil.BlockValid | 0x0008, true},
	}

	params := chaincfg.MainNetParams()
	for _, test := range tests {
		isValid := isValidVoteBits(params, test.voteBits)
		if isValid != test.isValid {
			t.Fatalf("isValidVoteBits failed for votebits '%d': want %v, got %v",
				test.voteBits, test.isValid, isValid)
		}
	}
}
