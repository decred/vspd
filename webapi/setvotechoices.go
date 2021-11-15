// Copyright (c) 2020-2021 The Decred developers
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

// setVoteChoices is the handler for "POST /api/v3/setvotechoices".
func setVoteChoices(c *gin.Context) {
	const funcName = "setVoteChoices"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet(ticketKey).(database.Ticket)
	knownTicket := c.MustGet(knownTicketKey).(bool)
	walletClients := c.MustGet(walletsKey).([]*rpc.WalletRPC)
	reqBytes := c.MustGet(requestBytesKey).([]byte)

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
		var prevReq struct {
			Timestamp int64 `json:"timestamp" binding:"required"`
		}
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

	err = validConsensusVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), request.VoteChoices)
	if err != nil {
		log.Warnf("%s: Invalid consensus vote choices (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendErrorWithMsg(err.Error(), errInvalidVoteChoices, c)
		return
	}

	// Update VoteChoices in the database before updating the wallets. DB is the
	// source of truth, and also is less likely to error.
	ticket.VoteChoices = request.VoteChoices

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

		// Just log any errors which occur while setting vote choices. We want
		// to attempt to update as much as possible regardless of any errors.
		for _, walletClient := range walletClients {

			// Set consensus vote choices.
			for agenda, choice := range request.VoteChoices {
				err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
				if err != nil {
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
