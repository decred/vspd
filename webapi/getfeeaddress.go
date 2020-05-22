package webapi

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
)

// feeAddress is the handler for "POST /feeaddress".
func feeAddress(c *gin.Context) {

	ctx := c.Request.Context()

	reqBytes, err := c.GetRawData()
	if err != nil {
		log.Warnf("Error reading request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	var feeAddressRequest FeeAddressRequest
	if err := binding.JSON.BindBody(reqBytes, &feeAddressRequest); err != nil {
		log.Warnf("Bad feeaddress request from %s: %v", c.ClientIP(), err)
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
	ticket, ticketFound, err := db.GetTicketByHash(feeAddressRequest.TicketHash)
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
		commitmentAddress, err = fWalletClient.GetTicketCommitmentAddress(feeAddressRequest.TicketHash, cfg.NetParams)
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

	// VSP already knows this ticket, and has already issued it a fee address.
	if ticketFound {
		// If the expiry period has passed we need to issue a new fee.
		now := time.Now()
		expire := ticket.FeeExpiration
		VSPFee := ticket.VSPFee
		if now.After(time.Unix(ticket.FeeExpiration, 0)) {
			expire = now.Add(cfg.FeeAddressExpiration).Unix()
			VSPFee = cfg.VSPFee

			err = db.UpdateExpireAndFee(ticket.Hash, expire, VSPFee)
			if err != nil {
				log.Errorf("UpdateExpireAndFee error: %v", err)
				sendErrorResponse("database error", http.StatusInternalServerError, c)
				return
			}
		}
		sendJSONResponse(feeAddressResponse{
			Timestamp:  now.Unix(),
			Request:    feeAddressRequest,
			FeeAddress: ticket.FeeAddress,
			Fee:        VSPFee,
			Expiration: expire,
		}, c)

		return
	}

	// Beyond this point we are processing a new ticket which the VSP has not
	// seen before.

	txHash, err := chainhash.NewHashFromStr(feeAddressRequest.TicketHash)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// Ensure ticket exists and is mined.
	resp, err := fWalletClient.GetRawTransaction(txHash.String())
	if err != nil {
		log.Warnf("Could not retrieve tx %s for %s: %v", txHash, c.ClientIP(), err)
		sendErrorResponse("unknown transaction", http.StatusBadRequest, c)
		return
	}
	if resp.Confirmations < 2 || resp.BlockHeight < 0 {
		log.Warnf("Not enough confs for tx from %s", c.ClientIP())
		sendErrorResponse("transaction does not have minimum confirmations", http.StatusBadRequest, c)
		return
	}
	if resp.Confirmations > int64(uint32(cfg.NetParams.TicketMaturity)+cfg.NetParams.TicketExpiry) {
		log.Warnf("Too old tx from %s", c.ClientIP())
		sendErrorResponse("transaction too old", http.StatusBadRequest, c)
		return
	}

	msgHex, err := hex.DecodeString(resp.Hex)
	if err != nil {
		log.Errorf("Failed to decode tx: %v", err)
		sendErrorResponse("unable to decode transaction", http.StatusInternalServerError, c)
		return
	}

	msgTx := wire.NewMsgTx()
	if err = msgTx.FromBytes(msgHex); err != nil {
		log.Errorf("Failed to deserialize tx: %v", err)
		sendErrorResponse("failed to deserialize transaction", http.StatusInternalServerError, c)
		return
	}
	if !stake.IsSStx(msgTx) {
		log.Warnf("Non-ticket tx from %s", c.ClientIP())
		sendErrorResponse("transaction is not a ticket", http.StatusBadRequest, c)
		return
	}
	if len(msgTx.TxOut) != 3 {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// get blockheight and sdiff which is required by
	// txrules.StakePoolTicketFee, and store them in the database
	// for processing by payfee
	blockHeader, err := fWalletClient.GetBlockHeader(resp.BlockHash)
	if err != nil {
		log.Errorf("GetBlockHeader error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	// TODO: Generate this within dcrvsp without an RPC call?
	newAddress, err := fWalletClient.GetNewAddress(cfg.FeeAccountName)
	if err != nil {
		log.Errorf("GetNewAddress error: %v", err)
		sendErrorResponse("unable to generate fee address", http.StatusInternalServerError, c)
		return
	}

	now := time.Now()
	expire := now.Add(cfg.FeeAddressExpiration).Unix()

	dbTicket := database.Ticket{
		Hash:              txHash.String(),
		CommitmentAddress: commitmentAddress,
		FeeAddress:        newAddress,
		SDiff:             blockHeader.SBits,
		BlockHeight:       int64(blockHeader.Height),
		VSPFee:            cfg.VSPFee,
		FeeExpiration:     expire,
		// VotingKey and VoteChoices: set during payfee
	}

	err = db.InsertTicket(dbTicket)
	if err != nil {
		log.Errorf("InsertTicket error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	sendJSONResponse(feeAddressResponse{
		Timestamp:  now.Unix(),
		Request:    feeAddressRequest,
		FeeAddress: newAddress,
		Fee:        cfg.VSPFee,
		Expiration: expire,
	}, c)
}
