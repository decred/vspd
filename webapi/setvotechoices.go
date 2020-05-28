package webapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/vspd/database"
	"github.com/jholdstock/vspd/rpc"
)

// setVoteChoices is the handler for "POST /setvotechoices".
func setVoteChoices(c *gin.Context) {

	// Get values which have been added to context by middleware.
	rawRequest := c.MustGet("RawRequest").([]byte)
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	walletClients := c.MustGet("WalletClient").([]*rpc.WalletRPC)

	if !knownTicket {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// TODO: Return an error if we dont have a FeeTx for this ticket yet.

	var setVoteChoicesRequest SetVoteChoicesRequest
	if err := binding.JSON.BindBody(rawRequest, &setVoteChoicesRequest); err != nil {
		log.Warnf("Bad setvotechoices request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	voteChoices := setVoteChoicesRequest.VoteChoices
	err := isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("Invalid votechoices from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// Update VoteChoices in the database before updating the wallets. DB is
	// source of truth and is less likely to error.
	ticket.VoteChoices = voteChoices
	err = db.UpdateTicket(ticket)
	if err != nil {
		log.Errorf("UpdateTicket error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// Update vote choices on voting wallets. Tickets are only added to voting
	// wallets if their fee is confirmed.
	if ticket.FeeConfirmed {
		for agenda, choice := range voteChoices {
			for _, walletClient := range walletClients {
				err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
				// TODO: This will error if the wallet does not know about the ticket yet.
				// TODO: We shouldn't return on first error - we want to try all wallets.
				if err != nil {
					log.Errorf("SetVoteChoice failed: %v", err)
					sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
					return
				}
			}
		}
	}

	log.Debugf("Vote choices updated for ticket: ticketHash=%s", ticket.Hash)

	// TODO: DB - error if given timestamp is older than any previous requests

	// TODO: DB - store setvotechoices receipt in log

	sendJSONResponse(setVoteChoicesResponse{
		Timestamp: time.Now().Unix(),
		Request:   setVoteChoicesRequest,
	}, c)
}
