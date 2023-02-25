// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"net/http"

	"github.com/decred/vspd/database"
	"github.com/gin-gonic/gin"
)

// ticketPage is the handler for "GET /ticket"
func (s *Server) ticketPage(c *gin.Context) {
	c.HTML(http.StatusOK, "ticket.html", gin.H{
		"WebApiCache": s.cache.getData(),
		"WebApiCfg":   s.cfg,
	})
}

// ticketErrPage returns error message to the ticket page.
func (s *Server) ticketErrPage(c *gin.Context, status int, message string) {
	c.HTML(status, "ticket.html", gin.H{
		"WebApiCache": s.cache.getData(),
		"WebApiCfg":   s.cfg,
		"Error":       message,
	})

}

// manualTicketSearch is the handler for "POST /ticket".
func (s *Server) manualTicketSearch(c *gin.Context) {
	// Get values which have been added to context by middleware.
	err := c.MustGet(errorKey)
	if err != nil {
		apiErr := err.(apiError)
		s.ticketErrPage(c, apiErr.httpStatus(), apiErr.String())
		return
	}

	ticket := c.MustGet(ticketKey).(database.Ticket)
	knownTicket := c.MustGet(knownTicketKey).(bool)

	voteChanges, err := s.db.GetVoteChanges(ticket.Hash)
	if err != nil {
		s.log.Errorf("db.GetVoteChanges error (ticket=%s): %v", ticket.Hash, err)
		s.ticketErrPage(c, http.StatusBadRequest, "Error getting vote changes from database")
		return
	}

	altSignAddrData, err := s.db.AltSignAddrData(ticket.Hash)
	if err != nil {
		s.log.Errorf("db.AltSignAddrData error (ticket=%s): %v", ticket.Hash, err)
		s.ticketErrPage(c, http.StatusBadRequest, "Error getting alternate signature from database")
		return
	}

	c.HTML(http.StatusOK, "ticket.html", gin.H{
		"SearchResult": searchResult{
			Hash:            ticket.Hash,
			Found:           knownTicket,
			Ticket:          ticket,
			AltSignAddrData: altSignAddrData,
			VoteChanges:     voteChanges,
			MaxVoteChanges:  s.cfg.MaxVoteChangeRecords,
		},
		"WebApiCache": s.cache.getData(),
		"WebApiCfg":   s.cfg,
	})
}
