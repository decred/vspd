package webapi

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/gin-gonic/gin"
)

// ticketStatus is the handler for "GET /ticketstatus"
func ticketStatus(c *gin.Context) {
	var ticketStatusRequest TicketStatusRequest
	if err := c.ShouldBindJSON(&ticketStatusRequest); err != nil {
		log.Warnf("Bad ticketstatus request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := ticketStatusRequest.TicketHash
	_, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := ticketStatusRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	ticket, err := db.GetTicketByHash(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 ticketstatus %d %s", ticketStatusRequest.Timestamp, ticketHashStr)
	err = dcrutil.VerifyMessage(ticket.CommitmentAddress, signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	sendJSONResponse(ticketStatusResponse{
		Timestamp:   time.Now().Unix(),
		Request:     ticketStatusRequest,
		Status:      "active", // TODO - active, pending, expired (missed, revoked?)
		VoteChoices: ticket.VoteChoices,
	}, c)
}
