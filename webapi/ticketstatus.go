package webapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/rpc"
)

// ticketStatus is the handler for "GET /ticketstatus".
func ticketStatus(c *gin.Context) {

	ctx := c.Request.Context()

	reqBytes, err := c.GetRawData()
	if err != nil {
		log.Warnf("Error reading request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	var ticketStatusRequest TicketStatusRequest
	if err := binding.JSON.BindBody(reqBytes, &ticketStatusRequest); err != nil {
		log.Warnf("Bad ticketstatus request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// Create a fee wallet client.
	fWalletConn, err := feeWalletConnect()
	if err != nil {
		log.Errorf("Fee wallet connection error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}
	fWalletClient, err := rpc.FeeWalletClient(ctx, fWalletConn)
	if err != nil {
		log.Errorf("Fee wallet client error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	// Check if this ticket already appears in the database.
	ticket, ticketFound, err := db.GetTicketByHash(ticketStatusRequest.TicketHash)
	if err != nil {
		log.Errorf("GetTicketByHash error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// If the ticket was found in the database we already know its commitment
	// address. Otherwise we need to get it from the chain.
	var commitmentAddress string
	if ticketFound {
		commitmentAddress = ticket.CommitmentAddress
	} else {
		commitmentAddress, err = fWalletClient.GetTicketCommitmentAddress(ticketStatusRequest.TicketHash, cfg.NetParams)
		if err != nil {
			log.Errorf("GetTicketCommitmentAddress error: %v", err)
			sendErrorResponse("database error", http.StatusInternalServerError, c)
			return
		}
	}

	// Validate request signature to ensure ticket ownership.
	err = validateSignature(reqBytes, commitmentAddress, c)
	if err != nil {
		log.Warnf("Bad signature from %s: %v", c.ClientIP(), err)
		sendErrorResponse("bad signature", http.StatusBadRequest, c)
		return
	}

	if !ticketFound {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	sendJSONResponse(ticketStatusResponse{
		Timestamp:   time.Now().Unix(),
		Request:     ticketStatusRequest,
		Status:      "active", // TODO - active, pending, expired (missed, revoked?)
		VoteChoices: ticket.VoteChoices,
	}, c)
}
