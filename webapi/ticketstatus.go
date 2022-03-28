// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	"github.com/decred/vspd/database"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// ticketStatus is the handler for "POST /api/v3/ticketstatus".
func (s *Server) ticketStatus(c *gin.Context) {
	const funcName = "ticketStatus"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet(ticketKey).(database.Ticket)
	knownTicket := c.MustGet(knownTicketKey).(bool)
	reqBytes := c.MustGet(requestBytesKey).([]byte)

	if !knownTicket {
		log.Warnf("%s: Unknown ticket (clientIP=%s)", funcName, c.ClientIP())
		s.sendError(errUnknownTicket, c)
		return
	}

	var request ticketStatusRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	// Get altSignAddress from database
	altSignAddrData, err := s.db.AltSignAddrData(ticket.Hash)
	if err != nil {
		log.Errorf("%s: db.AltSignAddrData error (ticketHash=%s): %v", funcName, ticket.Hash, err)
		s.sendError(errInternalError, c)
		return
	}

	altSignAddr := ""
	if altSignAddrData != nil {
		altSignAddr = altSignAddrData.AltSignAddr
	}

	s.sendJSONResponse(ticketStatusResponse{
		Timestamp:       time.Now().Unix(),
		Request:         reqBytes,
		TicketConfirmed: ticket.Confirmed,
		FeeTxStatus:     string(ticket.FeeTxStatus),
		FeeTxHash:       ticket.FeeTxHash,
		AltSignAddress:  altSignAddr,
		VoteChoices:     ticket.VoteChoices,
		TreasuryPolicy:  ticket.TreasuryPolicy,
		TSpendPolicy:    ticket.TSpendPolicy,
	}, c)
}
