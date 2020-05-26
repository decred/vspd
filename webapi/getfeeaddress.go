package webapi

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
)

// addrMtx protects getNewFeeAddress.
var addrMtx sync.Mutex

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
	dcrdClient := c.MustGet("DcrdClient").(*rpc.DcrdRPC)

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
		if ticket.FeeExpired() {
			ticket.FeeExpiration = now.Add(cfg.FeeAddressExpiration).Unix()
			ticket.VSPFee = cfg.VSPFee

			err := db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("UpdateTicket error: %v", err)
				sendErrorResponse("database error", http.StatusInternalServerError, c)
				return
			}
		}
		sendJSONResponse(feeAddressResponse{
			Timestamp:  now.Unix(),
			Request:    feeAddressRequest,
			FeeAddress: ticket.FeeAddress,
			Fee:        ticket.VSPFee,
			Expiration: ticket.FeeExpiration,
		}, c)

		return
	}

	// Beyond this point we are processing a new ticket which the VSP has not
	// seen before.

	ticketHash := feeAddressRequest.TicketHash

	// Get transaction details.
	rawTx, err := dcrdClient.GetRawTransaction(ticketHash)
	if err != nil {
		log.Warnf("Could not retrieve tx %s for %s: %v", ticketHash, c.ClientIP(), err)
		sendErrorResponse("unknown transaction", http.StatusBadRequest, c)
		return
	}

	// Don't accept tickets which are too old.
	if rawTx.Confirmations > int64(uint32(cfg.NetParams.TicketMaturity)+cfg.NetParams.TicketExpiry) {
		log.Warnf("Too old tx from %s", c.ClientIP())
		sendErrorResponse("transaction too old", http.StatusBadRequest, c)
		return
	}

	// Check if ticket is fully confirmed.
	var confirmed bool
	if rawTx.Confirmations >= 6 {
		confirmed = true
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
		Confirmed:         confirmed,
		VSPFee:            cfg.VSPFee,
		FeeExpiration:     expire,
		// VotingKey and VoteChoices: set during payfee
	}

	err = db.InsertNewTicket(dbTicket)
	if err != nil {
		log.Errorf("InsertTicket error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	log.Debugf("Fee address created for new ticket: tktConfirmed=%t, feeAddrIdx=%d, "+
		"feeAddr=%s, ticketHash=%s", confirmed, newAddressIdx, newAddress, ticketHash)

	sendJSONResponse(feeAddressResponse{
		Timestamp:  now.Unix(),
		Request:    feeAddressRequest,
		FeeAddress: newAddress,
		Fee:        cfg.VSPFee,
		Expiration: expire,
	}, c)
}
