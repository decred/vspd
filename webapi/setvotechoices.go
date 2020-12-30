// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type timestampRequest struct {
	Timestamp int64 `json:"timestamp" binding:"required"`
}

// setVoteChoices is the handler for "POST /api/v3/setvotechoices".
func setVoteChoices(c *gin.Context) {
	const funcName = "setVoteChoices"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	walletClients := c.MustGet("WalletClients").([]*rpc.WalletRPC)
	reqBytes := c.MustGet("RequestBytes").([]byte)

	// If we cannot set the vote choices on at least one voting wallet right
	// now, don't update the database, just return an error.
	if len(walletClients) == 0 {
		sendError(errInternalError, c)
		return
	}

	if !knownTicket {
		log.Warnf("%s: Unknown ticket (clientIP=%s)", funcName, c.ClientIP())
		sendError(errUnknownTicket, c)
		return
	}

	if ticket.FeeTxStatus == database.NoFee {
		log.Warnf("%s: No fee tx for ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		sendError(errFeeNotReceived, c)
		return
	}

	// Only allow vote choices to be updated for live/immature tickets.
	if ticket.Outcome != "" {
		log.Warnf("%s: Ticket not eligible to vote (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		sendErrorWithMsg(fmt.Sprintf("ticket not eligible to vote (status=%s)", ticket.Outcome),
			errTicketCannotVote, c)
		return
	}

	var request setVoteChoicesRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	// Return an error if this request has a timestamp older than any previous
	// vote change requests. This is to prevent requests from being replayed.
	previousChanges, err := db.GetVoteChanges(ticket.Hash)
	if err != nil {
		log.Errorf("%s: db.GetVoteChanges error (ticketHash=%s): %v",
			funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}

	for _, change := range previousChanges {
		var prevReq timestampRequest
		err := json.Unmarshal([]byte(change.Request), &prevReq)
		if err != nil {
			log.Errorf("%s: Could not unmarshal vote change record (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
			sendError(errInternalError, c)
			return
		}

		if request.Timestamp <= prevReq.Timestamp {
			log.Warnf("%s: Request uses invalid timestamp, %d is not greater "+
				"than %d (ticketHash=%s)",
				funcName, request.Timestamp, prevReq.Timestamp, ticket.Hash)
			sendError(errInvalidTimestamp, c)
			return
		}
	}

	voteChoices := request.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("%s: Invalid vote choices (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendErrorWithMsg(err.Error(), errInvalidVoteChoices, c)
		return
	}

	// Update VoteChoices in the database before updating the wallets. DB is
	// source of truth and is less likely to error.
	ticket.VoteChoices = voteChoices
	err = db.UpdateTicket(ticket)
	if err != nil {
		log.Errorf("%s: db.UpdateTicket error, failed to set vote choices (ticketHash=%s): %v",
			funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}

	// Update vote choices on voting wallets. Tickets are only added to voting
	// wallets if their fee is confirmed.
	if ticket.FeeTxStatus == database.FeeConfirmed {
		for agenda, choice := range voteChoices {
			for _, walletClient := range walletClients {
				err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
				if err != nil {
					// If this fails, we still want to try the other wallets, so
					// don't return an error response, just log an error.
					log.Errorf("%s: dcrwallet.SetVoteChoice failed (wallet=%s, ticketHash=%s): %v",
						funcName, walletClient.String(), ticket.Hash, err)
				}
			}
		}
	}

	log.Debugf("%s: Vote choices updated (ticketHash=%s)", funcName, ticket.Hash)

	// Send success response to client.
	resp, respSig := sendJSONResponse(setVoteChoicesResponse{
		Timestamp: time.Now().Unix(),
		Request:   reqBytes,
	}, c)

	// Store a record of the vote choice change.
	err = db.SaveVoteChange(
		ticket.Hash,
		database.VoteChangeRecord{
			Request:           string(reqBytes),
			RequestSignature:  c.GetHeader("VSP-Client-Signature"),
			Response:          resp,
			ResponseSignature: respSig,
		})
	if err != nil {
		log.Errorf("%s: Failed to store vote change record (ticketHash=%s): %v", err)
	}
}
