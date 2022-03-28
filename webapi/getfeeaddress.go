// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"sync"
	"time"

	"decred.org/dcrwallet/v2/wallet/txrules"
	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// addrMtx protects getNewFeeAddress.
var addrMtx sync.Mutex

// getNewFeeAddress gets a new address from the address generator, and updates
// the last used address index in the database. In order to maintain consistency
// between the internal counter of address generator and the database, this func
// uses a mutex to ensure it is not run concurrently.
func (s *Server) getNewFeeAddress() (string, uint32, error) {
	addrMtx.Lock()
	defer addrMtx.Unlock()

	addr, idx, err := s.addrGen.NextAddress()
	if err != nil {
		return "", 0, err
	}

	err = s.db.SetLastAddressIndex(idx)
	if err != nil {
		return "", 0, err
	}

	return addr, idx, nil
}

// getCurrentFee returns the minimum fee amount a client should pay in order to
// register a ticket with the VSP at the current block height.
func (s *Server) getCurrentFee(dcrdClient *rpc.DcrdRPC) (dcrutil.Amount, error) {
	bestBlock, err := dcrdClient.GetBestBlockHeader()
	if err != nil {
		return 0, err
	}

	sDiff, err := dcrutil.NewAmount(bestBlock.SBits)
	if err != nil {
		return 0, err
	}

	// Using a hard-coded amount for relay fee is acceptable here because this
	// amount is never actually used to construct or broadcast transactions. It
	// is only used to calculate the fee charged for adding a ticket to the VSP.
	const defaultMinRelayTxFee = dcrutil.Amount(1e4)

	isDCP0010Active, err := dcrdClient.IsDCP0010Active()
	if err != nil {
		return 0, err
	}

	fee := txrules.StakePoolTicketFee(sDiff, defaultMinRelayTxFee,
		int32(bestBlock.Height), s.cfg.VSPFee, s.cfg.NetParams, isDCP0010Active)
	if err != nil {
		return 0, err
	}
	return fee, nil
}

// feeAddress is the handler for "POST /api/v3/feeaddress".
func (s *Server) feeAddress(c *gin.Context) {

	const funcName = "feeAddress"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet(ticketKey).(database.Ticket)
	knownTicket := c.MustGet(knownTicketKey).(bool)
	commitmentAddress := c.MustGet(commitmentAddressKey).(string)
	dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		log.Errorf("%s: could not get dcrd client: %v", funcName, dcrdErr.(error))
		s.sendError(errInternalError, c)
		return
	}
	reqBytes := c.MustGet(requestBytesKey).([]byte)

	if s.cfg.VspClosed {
		s.sendError(errVspClosed, c)
		return
	}

	var request feeAddressRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	ticketHash := request.TicketHash

	// Respond early if we already have the fee tx for this ticket.
	if knownTicket &&
		(ticket.FeeTxStatus == database.FeeReceieved ||
			ticket.FeeTxStatus == database.FeeBroadcast ||
			ticket.FeeTxStatus == database.FeeConfirmed) {
		log.Warnf("%s: Fee tx already received (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		s.sendError(errFeeAlreadyReceived, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticketHash)
	if err != nil {
		log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, ticketHash, s.cfg.NetParams)
	if err != nil {
		log.Errorf("%s: dcrd.CanTicketVote error (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}
	if !canVote {
		log.Warnf("%s: Unvotable ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticketHash)
		s.sendError(errTicketCannotVote, c)
		return
	}

	// VSP already knows this ticket and has already issued it a fee address.
	if knownTicket {

		// If the expiry period has passed we need to issue a new fee.
		now := time.Now()
		if ticket.FeeExpired() {
			newFee, err := s.getCurrentFee(dcrdClient)
			if err != nil {
				log.Errorf("%s: getCurrentFee error (ticketHash=%s): %v", funcName, ticket.Hash, err)
				s.sendError(errInternalError, c)
				return
			}
			ticket.FeeExpiration = now.Add(feeAddressExpiration).Unix()
			ticket.FeeAmount = int64(newFee)

			err = s.db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to update fee expiry (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				s.sendError(errInternalError, c)
				return
			}
			log.Debugf("%s: Expired fee updated (newFeeAmt=%s, ticketHash=%s)",
				funcName, newFee, ticket.Hash)
		}
		s.sendJSONResponse(feeAddressResponse{
			Timestamp:  now.Unix(),
			Request:    reqBytes,
			FeeAddress: ticket.FeeAddress,
			FeeAmount:  ticket.FeeAmount,
			Expiration: ticket.FeeExpiration,
		}, c)

		return
	}

	// Beyond this point we are processing a new ticket which the VSP has not
	// seen before.

	fee, err := s.getCurrentFee(dcrdClient)
	if err != nil {
		log.Errorf("%s: getCurrentFee error (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}

	newAddress, newAddressIdx, err := s.getNewFeeAddress()
	if err != nil {
		log.Errorf("%s: getNewFeeAddress error (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}

	now := time.Now()
	expire := now.Add(feeAddressExpiration).Unix()

	// Only set purchase height if the ticket already has 6 confs, otherwise its
	// purchase height may change due to reorgs.
	confirmed := false
	purchaseHeight := int64(0)
	if rawTicket.Confirmations >= requiredConfs {
		confirmed = true
		purchaseHeight = rawTicket.BlockHeight
	}

	dbTicket := database.Ticket{
		Hash:              ticketHash,
		PurchaseHeight:    purchaseHeight,
		CommitmentAddress: commitmentAddress,
		FeeAddressIndex:   newAddressIdx,
		FeeAddress:        newAddress,
		Confirmed:         confirmed,
		FeeAmount:         int64(fee),
		FeeExpiration:     expire,
		FeeTxStatus:       database.NoFee,
	}

	err = s.db.InsertNewTicket(dbTicket)
	if err != nil {
		log.Errorf("%s: db.InsertNewTicket failed (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}

	log.Debugf("%s: Fee address created for new ticket: (tktConfirmed=%t, feeAddrIdx=%d, "+
		"feeAddr=%s, feeAmt=%s, ticketHash=%s)",
		funcName, confirmed, newAddressIdx, newAddress, fee, ticketHash)

	s.sendJSONResponse(feeAddressResponse{
		Timestamp:  now.Unix(),
		Request:    reqBytes,
		FeeAddress: newAddress,
		FeeAmount:  int64(fee),
		Expiration: expire,
	}, c)
}
