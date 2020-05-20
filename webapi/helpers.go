package webapi

import (
	"fmt"

	"github.com/decred/dcrd/chaincfg/v3"
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

// isValidVoteChoices returns an error if provided vote choices are not valid for
// the most recent agendas.
func isValidVoteChoices(params *chaincfg.Params, voteVersion uint32, voteChoices map[string]string) error {

agendaLoop:
	for agenda, choice := range voteChoices {
		// Does the agenda exist?
		for _, v := range params.Deployments[voteVersion] {
			if v.Vote.Id == agenda {
				// Agenda exists - does the vote choice exist?
				for _, c := range v.Vote.Choices {
					if c.Id == choice {
						// Valid agenda and choice combo! Check the next one...
						continue agendaLoop
					}
				}
				return fmt.Errorf("choice %q not found for agenda %q", choice, agenda)
			}

		}
		return fmt.Errorf("agenda %q not found for vote version %d", agenda, voteVersion)
	}

	return nil
}
