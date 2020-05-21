package webapi

import (
	"testing"

	"github.com/decred/dcrd/chaincfg/v3"
)

func TestIsValidVoteChoices(t *testing.T) {

	// Mainnet vote version 4 contains 2 agendas - sdiffalgorithm and lnsupport.
	// Both agendas have vote choices yes/no/abstain.
	voteVersion := uint32(4)
	params := chaincfg.MainNetParams()

	var tests = []struct {
		voteChoices map[string]string
		valid       bool
	}{
		// Empty vote choices are allowed.
		{map[string]string{}, true},

		// Valid agenda, valid vote choice.
		{map[string]string{"lnsupport": "yes"}, true},
		{map[string]string{"sdiffalgorithm": "no", "lnsupport": "yes"}, true},

		// Invalid agenda.
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
		err := isValidVoteChoices(params, voteVersion, test.voteChoices)
		if (err == nil) != test.valid {
			t.Fatalf("isValidVoteChoices failed for votechoices '%v'.", test.voteChoices)
		}
	}
}
