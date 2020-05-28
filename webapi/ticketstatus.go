package webapi

import (
	"net/http"
	"time"

	"github.com/decred/vspd/database"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// ticketStatus is the handler for "GET /ticketstatus".
func ticketStatus(c *gin.Context) {

	// Get values which have been added to context by middleware.
	rawRequest := c.MustGet("RawRequest").([]byte)
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)

	if !knownTicket {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	var ticketStatusRequest TicketStatusRequest
	if err := binding.JSON.BindBody(rawRequest, &ticketStatusRequest); err != nil {
		log.Warnf("Bad ticketstatus request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	sendJSONResponse(ticketStatusResponse{
		Timestamp:       time.Now().Unix(),
		Request:         ticketStatusRequest,
		TicketConfirmed: ticket.Confirmed,
		FeeTxReceived:   ticket.FeeTxHex != "",
		FeeTxBroadcast:  ticket.FeeTxHash != "",
		FeeConfirmed:    ticket.FeeConfirmed,
		FeeTxHash:       ticket.FeeTxHash,
		VoteChoices:     ticket.VoteChoices,
	}, c)
}
