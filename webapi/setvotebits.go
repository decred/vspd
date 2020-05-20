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

// setVoteBits is the handler for "POST /setvotebits"
func setVoteBits(c *gin.Context) {
	var setVoteBitsRequest SetVoteBitsRequest
	if err := c.ShouldBindJSON(&setVoteBitsRequest); err != nil {
		log.Warnf("Bad setvotebits request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := setVoteBitsRequest.TicketHash
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := setVoteBitsRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	// votebits
	voteBits := setVoteBitsRequest.VoteBits
	if !isValidVoteBits(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteBits) {
		log.Warnf("Invalid votebits from %s", c.ClientIP())
		sendErrorResponse("invalid votebits", http.StatusBadRequest, c)
		return
	}

	ticket, err := db.GetTicketByHash(txHash.String())
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 setvotebits %d %s %d", setVoteBitsRequest.Timestamp, txHash, voteBits)
	err = dcrutil.VerifyMessage(ticket.CommitmentAddress, signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Failed to verify message from %s", c.ClientIP())
		sendErrorResponse("message did not pass verification", http.StatusBadRequest, c)
		return
	}

	err = db.UpdateVoteBits(txHash.String(), voteBits)
	if err != nil {
		log.Errorf("UpdateVoteBits error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// TODO: DB - error if given timestamp is older than any previous requests

	// TODO: DB - store setvotebits receipt in log

	sendJSONResponse(setVoteBitsResponse{
		Timestamp: time.Now().Unix(),
		Request:   setVoteBitsRequest,
		VoteBits:  voteBits,
	}, c)
}
