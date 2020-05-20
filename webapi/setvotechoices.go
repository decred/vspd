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

// setVoteChoices is the handler for "POST /setvotechoices"
func setVoteChoices(c *gin.Context) {
	var setVoteChoicesRequest SetVoteChoicesRequest
	if err := c.ShouldBindJSON(&setVoteChoicesRequest); err != nil {
		log.Warnf("Bad setvotechoices request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := setVoteChoicesRequest.TicketHash
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := setVoteChoicesRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s: %v", c.ClientIP(), err)
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	voteChoices := setVoteChoicesRequest.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("Invalid votechoices from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	ticket, err := db.GetTicketByHash(txHash.String())
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 setvotechoices %d %s %v", setVoteChoicesRequest.Timestamp, txHash, voteChoices)
	err = dcrutil.VerifyMessage(ticket.CommitmentAddress, signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Failed to verify message from %s: %v", c.ClientIP(), err)
		sendErrorResponse("message did not pass verification", http.StatusBadRequest, c)
		return
	}

	walletClient, err := walletRPC()
	if err != nil {
		log.Errorf("Failed to dial dcrwallet RPC: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	ctx := c.Request.Context()

	// Update vote choices on voting wallets.
	for agenda, choice := range voteChoices {
		err = walletClient.Call(ctx, "setvotechoice", nil, agenda, choice, ticket.Hash)
		if err != nil {
			log.Errorf("setvotechoice failed: %v", err)
			sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
			return
		}
	}

	err = db.UpdateVoteChoices(txHash.String(), voteChoices)
	if err != nil {
		log.Errorf("UpdateVoteChoices error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// TODO: DB - error if given timestamp is older than any previous requests

	// TODO: DB - store setvotechoices receipt in log

	sendJSONResponse(setVoteChoicesResponse{
		Timestamp:   time.Now().Unix(),
		Request:     setVoteChoicesRequest,
		VoteChoices: voteChoices,
	}, c)
}
