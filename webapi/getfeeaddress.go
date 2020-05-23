package webapi

import (
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
)

// addrMtx protects getNewFeeAddress.
var addrMtx sync.RWMutex

// getNewFeeAddress gets a new address from the address generator and stores the
// new address index in the database. In order to maintain consistency between
// the internal counter of address generator and the database, this function
// cannot be run concurrently.
func getNewFeeAddress(db *database.VspDatabase, addrGen *addressGenerator) (string, uint32, error) {
	addrMtx.Lock()
	defer addrMtx.Unlock()

	addr, idx, err := addrGen.NextAddress()
	if err != nil {
		return "", 0, err
	}

	err = db.SetLastAddressIndex(idx)
	if err != nil {
		return "", 0, err
	}

	return addr, idx, nil
}

// feeAddress is the handler for "POST /feeaddress".
func feeAddress(c *gin.Context) {

	// Get values which have been added to context by middleware.
	rawRequest := c.MustGet("RawRequest").([]byte)
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	commitmentAddress := c.MustGet("CommitmentAddress").(string)
	fWalletClient := c.MustGet("FeeWalletClient").(*rpc.FeeWalletRPC)

	var feeAddressRequest FeeAddressRequest
	if err := binding.JSON.BindBody(rawRequest, &feeAddressRequest); err != nil {
		log.Warnf("Bad feeaddress request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// VSP already knows this ticket and has already issued it a fee address.
	if knownTicket {
		// If the expiry period has passed we need to issue a new fee.
		now := time.Now()
		expire := ticket.FeeExpiration
		VSPFee := ticket.VSPFee
		if now.After(time.Unix(ticket.FeeExpiration, 0)) {
			expire = now.Add(cfg.FeeAddressExpiration).Unix()
			VSPFee = cfg.VSPFee

			err := db.UpdateExpireAndFee(ticket.Hash, expire, VSPFee)
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

	ticketHash := feeAddressRequest.TicketHash

	// Ensure ticket exists and is mined.
	resp, err := fWalletClient.GetRawTransaction(ticketHash)
	if err != nil {
		log.Warnf("Could not retrieve tx %s for %s: %v", ticketHash, c.ClientIP(), err)
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

	newAddress, newAddressIdx, err := getNewFeeAddress(db, addrGen)
	if err != nil {
		log.Errorf("getNewFeeAddress error: %v", err)
	}

	now := time.Now()
	expire := now.Add(cfg.FeeAddressExpiration).Unix()

	dbTicket := database.Ticket{
		Hash:              ticketHash,
		CommitmentAddress: commitmentAddress,
		FeeAddressIndex:   newAddressIdx,
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

	log.Debugf("Fee address created for new ticket: feeAddrIdx=%d, "+
		"feeAddr=%s, ticketHash=%s", newAddressIdx, newAddress, ticketHash)

	sendJSONResponse(feeAddressResponse{
		Timestamp:  now.Unix(),
		Request:    feeAddressRequest,
		FeeAddress: newAddress,
		Fee:        cfg.VSPFee,
		Expiration: expire,
	}, c)
}
