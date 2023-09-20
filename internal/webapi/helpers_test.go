// Copyright (c) 2020-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"testing"

	"github.com/decred/vspd/internal/config"
)

func TestIsValidVoteChoices(t *testing.T) {

	// Mainnet vote version 4 contains 2 agendas - sdiffalgorithm and lnsupport.
	// Both agendas have vote choices yes/no/abstain.
	voteVersion := uint32(4)
	network := config.MainNet

	var tests = []struct {
		voteChoices map[string]string
		valid       bool
	}{
		// Empty vote choices are allowed.
		{map[string]string{}, true},

		// Valid agenda, valid vote choice.
		{map[string]string{"lnsupport": "yes"}, true},
		{map[string]string{"sdiffalgorithm": "no", "lnsupport": "yes"}, true},

		// Invalid agenda, valid vote choice.
		{map[string]string{"": "yes"}, false},
		{map[string]string{"Fake agenda": "yes"}, false},

		// Valid agenda, invalid vote choice.
		{map[string]string{"lnsupport": "1234"}, false},
		{map[string]string{"sdiffalgorithm": ""}, false},

		// One valid choice, one invalid choice.
		{map[string]string{"sdiffalgorithm": "no", "lnsupport": "1234"}, false},
		{map[string]string{"sdiffalgorithm": "1234", "lnsupport": "no"}, false},

		// One valid agenda, one invalid agenda.
		{map[string]string{"fake": "abstain", "lnsupport": "no"}, false},
		{map[string]string{"sdiffalgorithm": "abstain", "": "no"}, false},
	}

	for _, test := range tests {
		err := validConsensusVoteChoices(&network, voteVersion, test.voteChoices)
		if (err == nil) != test.valid {
			t.Fatalf("isValidVoteChoices failed for votechoices '%v': %v",
				test.voteChoices, err)
		}
	}
}

func TestIsValidTSpendPolicy(t *testing.T) {

	// A valid tspend hash is 32 bytes (64 characters).
	const validHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const anotherValidHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	var tests = []struct {
		tspendPolicy map[string]string
		valid        bool
	}{
		// Empty vote choices are allowed.
		{map[string]string{}, true},

		// Valid tspend hash, valid vote choice.
		{map[string]string{validHash: "yes"}, true},
		{map[string]string{validHash: ""}, true},
		{map[string]string{validHash: "no", anotherValidHash: "yes"}, true},

		// Invalid tspend hash.
		{map[string]string{"": "yes"}, false},
		{map[string]string{"a": "yes"}, false},
		{map[string]string{"non hex characters": "yes"}, false},
		{map[string]string{validHash + "a": "yes"}, false},

		// Valid tspend hash, invalid vote choice.
		{map[string]string{validHash: "1234"}, false},

		// // One valid choice, one invalid choice.
		{map[string]string{validHash: "no", anotherValidHash: "1234"}, false},
		{map[string]string{validHash: "1234", anotherValidHash: "no"}, false},

		// One valid tspend hash, one invalid tspend hash.
		{map[string]string{"fake": "abstain", anotherValidHash: "no"}, false},
		{map[string]string{validHash: "abstain", "": "no"}, false},
	}

	for _, test := range tests {
		err := validTSpendPolicy(test.tspendPolicy)
		if (err == nil) != test.valid {
			t.Fatalf("validTSpendPolicy failed for policy '%v': %v",
				test.tspendPolicy, err)
		}
	}
}

func TestIsValidTreasuryPolicy(t *testing.T) {

	// A valid treasury key is 33 bytes (66 characters).
	const validKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const anotherValidKey = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	var tests = []struct {
		treasuryPolicy map[string]string
		valid          bool
	}{
		// Empty vote choices are allowed.
		{map[string]string{}, true},

		// Valid treasury key, valid vote choice.
		{map[string]string{validKey: "yes"}, true},
		{map[string]string{validKey: ""}, true},
		{map[string]string{validKey: "no", anotherValidKey: "yes"}, true},

		// Invalid treasury key.
		{map[string]string{"": "yes"}, false},
		{map[string]string{"a": "yes"}, false},
		{map[string]string{"non hex characters": "yes"}, false},
		{map[string]string{validKey + "a": "yes"}, false},

		// Valid treasury key, invalid vote choice.
		{map[string]string{validKey: "1234"}, false},

		// // One valid choice, one invalid choice.
		{map[string]string{validKey: "no", anotherValidKey: "1234"}, false},
		{map[string]string{validKey: "1234", anotherValidKey: "no"}, false},

		// One valid treasury key, one invalid treasury key.
		{map[string]string{"fake": "abstain", anotherValidKey: "no"}, false},
		{map[string]string{validKey: "abstain", "": "no"}, false},
	}

	for _, test := range tests {
		err := validTreasuryPolicy(test.treasuryPolicy)
		if (err == nil) != test.valid {
			t.Fatalf("validTreasuryPolicy failed for policy '%v': %v",
				test.treasuryPolicy, err)
		}
	}
}
