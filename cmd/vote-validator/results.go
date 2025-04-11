// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
)

type results struct {
	badVotes      []*votedTicket
	noPreferences []*votedTicket
	wrongVersion  []*votedTicket
}

func (r *results) writeFile(path string) (bool, error) {

	if len(r.badVotes) == 0 &&
		len(r.noPreferences) == 0 &&
		len(r.wrongVersion) == 0 {
		return false, nil
	}

	// Open a log file.
	f, err := os.Create(path)
	if err != nil {
		return false, fmt.Errorf("opening log file failed: %w", err)
	}

	write := func(f *os.File, format string, a ...any) {
		_, err := fmt.Fprintf(f, format+"\n", a...)
		if err != nil {
			f.Close()
			panic(fmt.Sprintf("writing to log file failed: %v", err))
		}
	}

	if len(r.badVotes) > 0 {
		write(f, "Tickets with bad votes:")
		for _, t := range r.badVotes {
			write(f,
				"Hash: %s VoteHeight: %d ExpectedVote: %v ActualVote: %v",
				t.ticket.Hash, t.voteHeight, t.ticket.VoteChoices, t.vote,
			)
		}
		write(f, "\n")
	}

	if len(r.wrongVersion) > 0 {
		write(f, "Tickets with the wrong vote version:")
		for _, t := range r.wrongVersion {
			write(f,
				"Hash: %s",
				t.ticket.Hash,
			)
		}
		write(f, "\n")
	}

	if len(r.noPreferences) > 0 {
		write(f, "Tickets with no user set vote preferences:")
		for _, t := range r.noPreferences {
			write(f,
				"Hash: %s",
				t.ticket.Hash,
			)
		}
		write(f, "\n")
	}

	err = f.Close()
	if err != nil {
		return false, fmt.Errorf("closing log file failed: %w", err)
	}

	return true, nil
}
