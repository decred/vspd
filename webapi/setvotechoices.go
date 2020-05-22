package webapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/rpc"
)

// setVoteChoices is the handler for "POST /setvotechoices".
func setVoteChoices(c *gin.Context) {

	ctx := c.Request.Context()

	reqBytes, err := c.GetRawData()
	if err != nil {
		log.Warnf("Error reading request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	var setVoteChoicesRequest SetVoteChoicesRequest
	if err := binding.JSON.BindBody(reqBytes, &setVoteChoicesRequest); err != nil {
		log.Warnf("Bad setvotechoices request from %s: %v", c.ClientIP(), err)
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
	ticket, ticketFound, err := db.GetTicketByHash(setVoteChoicesRequest.TicketHash)
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
		commitmentAddress, err = fWalletClient.GetTicketCommitmentAddress(setVoteChoicesRequest.TicketHash, cfg.NetParams)
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

	voteChoices := setVoteChoicesRequest.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("Invalid votechoices from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	vWalletConn, err := votingWalletConnect()
	if err != nil {
		log.Errorf("Voting wallet connection error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	vWalletClient, err := rpc.VotingWalletClient(ctx, vWalletConn)
	if err != nil {
		log.Errorf("Voting wallet client error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	// Update VoteChoices in the database before updating the wallets. DB is
	// source of truth and is less likely to error.
	err = db.UpdateVoteChoices(ticket.Hash, voteChoices)
	if err != nil {
		log.Errorf("UpdateVoteChoices error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// Update vote choices on voting wallets.
	for agenda, choice := range voteChoices {
		err = vWalletClient.SetVoteChoice(agenda, choice, ticket.Hash)
		if err != nil {
			log.Errorf("SetVoteChoice failed: %v", err)
			sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
			return
		}
	}

	// TODO: DB - error if given timestamp is older than any previous requests

	// TODO: DB - store setvotechoices receipt in log

	sendJSONResponse(setVoteChoicesResponse{
		Timestamp:   time.Now().Unix(),
		Request:     setVoteChoicesRequest,
		VoteChoices: voteChoices,
	}, c)
}
