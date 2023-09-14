// Copyright (c) 2022-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"sort"

	"github.com/decred/slog"
	"github.com/jessevdk/go-flags"

	"github.com/decred/vspd/database"
	"github.com/decred/vspd/internal/config"
)

const (
	logPath = "./vote-validator.log"

	// Send tx hashes to dcrdata in "chunks" to avoid hitting request size limits.
	chunkSize = 2000
)

var cfg struct {
	Testnet      bool   `short:"t" long:"testnet" description:"Run testnet instead of mainnet"`
	ToCheck      int    `short:"n" long:"tickets_to_check" required:"true" description:"Validate votes of the n most recently voted tickets"`
	DatabaseFile string `short:"f" long:"database_file" required:"true" description:"Full path of database file"`
}

type votedTicket struct {
	// From vspd db.
	ticket database.Ticket
	// From dcrdata.
	voteHeight  uint32
	voteVersion uint32
	vote        map[string]string
}

func main() {
	// Run until an exit code is returned.
	os.Exit(run())
}

func run() int {
	// Load config, display help if requested.
	_, err := flags.Parse(&cfg)
	if err != nil {
		var e *flags.Error
		if errors.As(err, &e) {
			if e.Type == flags.ErrHelp {
				return 0
			}
		}
		return 1
	}

	var network *config.Network
	if cfg.Testnet {
		network = &config.TestNet3
	} else {
		network = &config.MainNet
	}

	dcrdata := &dcrdataClient{URL: network.BlockExplorerURL}

	// Get the latest vote version. Any votes which don't match this version
	// will be ignored.
	var latestVoteVersion uint32
	for version := range network.Deployments {
		if version > latestVoteVersion {
			latestVoteVersion = version
		}
	}

	// Open database.
	log := slog.NewBackend(os.Stdout).Logger("")
	vdb, err := database.Open(cfg.DatabaseFile, log, 999)
	if err != nil {
		log.Error(err)
		return 1
	}

	// Get all voted tickets from database.
	dbTickets, err := vdb.GetVotedTickets()
	if err != nil {
		log.Error(err)
		return 1
	}

	numTickets := len(dbTickets)
	log.Infof("Database has %d voted tickets", numTickets)

	// A bit of pre-processing for later:
	//   - Store the tickets in a map using their hash as the key. This makes
	//     it easier to reference them later.
	//   - Create an array of all hashes. This can easily be sliced into
	//     "chunks" and sent to dcrdata.

	ticketMap := make(map[string]*votedTicket, numTickets)
	ticketHashes := make([]string, 0)
	for _, t := range dbTickets {
		ticketMap[t.Hash] = &votedTicket{ticket: t}
		ticketHashes = append(ticketHashes, t.Hash)
	}

	// Use dcrdata to get spender info for voted tickets (dcrd can't do this).
	log.Infof("Getting vote info from %s", dcrdata.URL)
	for i := 0; i < numTickets; i += chunkSize {
		end := i + chunkSize
		if end > numTickets {
			end = numTickets
		}

		// Get the tx info for each ticket.
		ticketTxns, err := dcrdata.txns(ticketHashes[i:end], true)
		if err != nil {
			log.Error(err)
			return 1
		}

		spenderHashes := make([]string, 0)
		mapSpenderToTicket := make(map[string]string, len(ticketTxns))
		for _, txn := range ticketTxns {
			spenderHash := txn.Vout[0].Spend.Hash
			spenderHashes = append(spenderHashes, spenderHash)
			mapSpenderToTicket[spenderHash] = txn.TxID
		}

		spenderTxns, err := dcrdata.txns(spenderHashes, false)
		if err != nil {
			log.Error(err)
			return 1
		}

		for _, tx := range spenderTxns {
			ticketHash := mapSpenderToTicket[tx.TxID]

			// Extract vote height from vOut[0]
			vOut0Script, err := hex.DecodeString(tx.Vout[0].ScriptPubKeyDecoded.Hex)
			if err != nil {
				log.Error(err)
				return 1
			}
			// dcrd/blockchain/stake/staketx.go - SSGenBlockVotedOn()
			votedHeight := binary.LittleEndian.Uint32(vOut0Script[34:38])

			// Extract vote version and votebits from vOut[1]
			vOut1Script, err := hex.DecodeString(tx.Vout[1].ScriptPubKeyDecoded.Hex)
			if err != nil {
				log.Error(err)
				return 1
			}
			// dcrd/blockchain/stake/staketx.go - SSGenVersion()
			voteVersion := binary.LittleEndian.Uint32(vOut1Script[4:8])

			// dcrd/blockchain/stake/staketx.go - SSGenVoteBits()
			votebits := binary.LittleEndian.Uint16(vOut1Script[2:4])

			// Get the recorded on-chain votes for this ticket.
			actualVote := make(map[string]string)
			agendas := network.Deployments[latestVoteVersion]
			for _, agenda := range agendas {
				for _, choice := range agenda.Vote.Choices {
					if votebits&agenda.Vote.Mask == choice.Bits {
						actualVote[agenda.Vote.Id] = choice.Id
					}
				}
			}

			ticketMap[ticketHash].voteHeight = votedHeight
			ticketMap[ticketHash].voteVersion = voteVersion
			ticketMap[ticketHash].vote = actualVote
		}

		log.Infof(" %6d of %d (%0.2f%%)", end, numTickets, float32(end)/float32(numTickets)*100)
	}

	// Convert ticketMap into a slice so that it can be sorted.
	sorted := make([]*votedTicket, 0)
	for _, ticket := range ticketMap {
		sorted = append(sorted, ticket)
	}

	// Sort tickets by vote height
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].voteHeight > sorted[j].voteHeight
	})

	// Do the checks.
	results := &results{
		badVotes:      make([]*votedTicket, 0),
		noPreferences: make([]*votedTicket, 0),
		wrongVersion:  make([]*votedTicket, 0),
	}

	for _, t := range sorted[0:cfg.ToCheck] {

		if t.voteVersion != latestVoteVersion {
			results.wrongVersion = append(results.wrongVersion, t)
			continue
		}

		// Get the vote preferences requested by the user.
		requestedVote := t.ticket.VoteChoices

		if len(requestedVote) == 0 {
			results.noPreferences = append(results.noPreferences, t)
		}

		badVote := false
		for agenda, actualChoice := range t.vote {
			reqChoice, ok := requestedVote[agenda]
			// If no choice set, should be abstain.
			if !ok {
				reqChoice = "abstain"
			}

			if actualChoice != reqChoice {
				badVote = true
			}
		}

		if badVote {
			results.badVotes = append(results.badVotes, t)
		}
	}

	log.Infof("")
	log.Infof("Checked %d most recently voted tickets", cfg.ToCheck)
	log.Infof(" %6d tickets had incorrect votes", len(results.badVotes))
	log.Infof(" %6d tickets not checked due to wrong vote version", len(results.wrongVersion))
	log.Infof(" %6d tickets had no voting preferences set by user", len(results.noPreferences))

	written, err := results.writeFile(logPath)
	if err != nil {
		log.Errorf("Failed to write log file: %v", err)
		return 1
	}
	if written {
		log.Infof("")
		log.Infof("Detailed information written to %s", logPath)
	}

	return 0
}
