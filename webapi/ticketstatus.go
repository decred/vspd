package webapi

import (
	"time"

	"github.com/decred/vspd/database"
	"github.com/gin-gonic/gin"
)

// ticketStatus is the handler for "GET /ticketstatus".
func ticketStatus(c *gin.Context) {
	funcName := "ticketStatus"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)

	if !knownTicket {
		log.Warnf("%s: Unknown ticket from %s", funcName, c.ClientIP())
		sendError(errUnknownTicket, c)
		return
	}

	var ticketStatusRequest TicketStatusRequest
	if err := c.ShouldBindJSON(&ticketStatusRequest); err != nil {
		log.Warnf("%s: Bad request from %s: %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	sendJSONResponse(ticketStatusResponse{
		Timestamp:       time.Now().Unix(),
		Request:         ticketStatusRequest,
		TicketConfirmed: ticket.Confirmed,
		FeeTxStatus:     string(ticket.FeeTxStatus),
		FeeTxHash:       ticket.FeeTxHash,
		VoteChoices:     ticket.VoteChoices,
	}, c)
}
