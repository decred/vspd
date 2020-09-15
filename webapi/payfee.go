// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	"github.com/decred/dcrd/blockchain/v3"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
)

// payFee is the handler for "POST /api/v3/payfee".
func payFee(c *gin.Context) {
	const funcName = "payFee"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	dcrdClient := c.MustGet("DcrdClient").(*rpc.DcrdRPC)

	if cfg.VspClosed {
		sendError(errVspClosed, c)
		return
	}

	if !knownTicket {
		log.Warnf("%s: Unknown ticket (clientIP=%s)", funcName, c.ClientIP())
		sendError(errUnknownTicket, c)
		return
	}

	var request payFeeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	// Respond early if we already have the fee tx for this ticket.
	if ticket.FeeTxStatus == database.FeeReceieved ||
		ticket.FeeTxStatus == database.FeeBroadcast ||
		ticket.FeeTxStatus == database.FeeConfirmed {
		log.Warnf("%s: Fee tx already received (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		sendError(errFeeAlreadyReceived, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
	if err != nil {
		log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v", funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, ticket.Hash, cfg.NetParams)
	if err != nil {
		log.Errorf("%s: dcrd.CanTicketVote error (ticketHash=%s): %v", funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}
	if !canVote {
		log.Warnf("%s: Unvotable ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		sendError(errTicketCannotVote, c)
		return
	}

	// Respond early if the fee for this ticket is expired.
	if ticket.FeeExpired() {
		log.Warnf("%s: Expired payfee request (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticket.Hash)
		sendError(errFeeExpired, c)
		return
	}

	// Validate VotingKey.
	votingKey := request.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		log.Warnf("%s: Failed to decode WIF (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendError(errInvalidPrivKey, c)
		return
	}

	// Validate VoteChoices.
	voteChoices := request.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("%s: Invalid vote choices (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendErrorWithMsg(err.Error(), errInvalidVoteChoices, c)
		return
	}

	// Validate FeeTx.
	feeTx, err := decodeTransaction(request.FeeTx)
	if err != nil {
		log.Warnf("%s: Failed to decode fee tx hex (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendError(errInvalidFeeTx, c)
		return
	}

	err = blockchain.CheckTransactionSanity(feeTx, cfg.NetParams)
	if err != nil {
		log.Warnf("%s: Fee tx failed sanity check (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), ticket.Hash, err)
		sendError(errInvalidFeeTx, c)
		return
	}

	// Loop through transaction outputs until we find one which pays to the
	// expected fee address. Record how much is being paid to the fee address.
	var feePaid dcrutil.Amount
	const scriptVersion = 0

findAddress:
	for _, txOut := range feeTx.TxOut {
		if txOut.Version != scriptVersion {
			log.Errorf("%s: Fee tx with invalid script version (clientIP=%s, ticketHash=%s): was %d, expected %d",
				funcName, c.ClientIP(), ticket.Hash, txOut.Version, scriptVersion)
			sendErrorWithMsg("invalid script version", errInvalidFeeTx, c)
			return
		}
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(scriptVersion,
			txOut.PkScript, cfg.NetParams)
		if err != nil {
			log.Errorf("%s: Extract PK error (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), ticket.Hash, err)
			sendError(errInternalError, c)
			return
		}
		for _, addr := range addresses {
			if addr.Address() == ticket.FeeAddress {
				feePaid = dcrutil.Amount(txOut.Value)
				break findAddress
			}
		}
	}

	if feePaid == 0 {
		log.Warnf("%s: Fee tx did not include expected payment (ticketHash=%s, feeAddress=%s, clientIP=%s)",
			funcName, ticket.Hash, ticket.FeeAddress, c.ClientIP())
		sendErrorWithMsg("feetx did not include any payments for fee address", errInvalidFeeTx, c)
		return
	}

	wifAddr, err := dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.NetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		log.Errorf("%s: Failed to get voting address from WIF (ticketHash=%s, clientIP=%s): %v",
			funcName, ticket.Hash, c.ClientIP(), err)
		sendError(errInvalidPrivKey, c)
		return
	}

	// Decode ticket transaction to get its voting address.
	ticketTx, err := decodeTransaction(rawTicket.Hex)
	if err != nil {
		log.Warnf("%s: Failed to decode ticket hex (ticketHash=%s): %v",
			funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}

	// Get ticket voting address.
	_, votingAddr, _, err := txscript.ExtractPkScriptAddrs(scriptVersion, ticketTx.TxOut[0].PkScript, cfg.NetParams)
	if err != nil {
		log.Errorf("%s: ExtractPK error (ticketHash=%s): %v", funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}
	if len(votingAddr) == 0 {
		log.Error("%s: No voting address found for ticket (ticketHash=%s)", funcName, ticket.Hash)
		sendError(errInternalError, c)
		return
	}

	// Ensure provided private key will allow us to vote this ticket.
	if votingAddr[0].Address() != wifAddr.Address() {
		log.Warnf("%s: Voting address does not match provided private key: (ticketHash=%s, votingAddr=%+v, wifAddr=%+v)",
			funcName, ticket.Hash, votingAddr[0], wifAddr)
		sendErrorWithMsg("voting address does not match provided private key",
			errInvalidPrivKey, c)
		return
	}

	minFee := dcrutil.Amount(ticket.FeeAmount)
	if feePaid < minFee {
		log.Warnf("%s: Fee too small (ticketHash=%s, clientIP=%s): was %s, expected minimum %s",
			funcName, ticket.Hash, c.ClientIP(), feePaid, minFee)
		sendError(errFeeTooSmall, c)
		return
	}

	// At this point we are satisfied that the request is valid and the fee tx
	// pays sufficient fees to the expected address. Proceed to update the
	// database, and if the ticket is confirmed broadcast the transaction.

	ticket.VotingWIF = votingWIF.String()
	ticket.FeeTxHex = request.FeeTx
	ticket.FeeTxHash = feeTx.TxHash().String()
	ticket.VoteChoices = voteChoices
	ticket.FeeTxStatus = database.FeeReceieved

	err = db.UpdateTicket(ticket)
	if err != nil {
		log.Errorf("%s: db.UpdateTicket error, failed to set fee tx (ticketHash=%s): %v",
			funcName, ticket.Hash, err)
		sendError(errInternalError, c)
		return
	}

	log.Debugf("%s: Fee tx received for ticket (minExpectedFee=%v, feePaid=%v, ticketHash=%s)",
		funcName, minFee, feePaid, ticket.Hash)

	if ticket.Confirmed {
		err = dcrdClient.SendRawTransaction(request.FeeTx)
		if err != nil {
			log.Errorf("%s: dcrd.SendRawTransaction for fee tx failed (ticketHash=%s): %v",
				funcName, ticket.Hash, err)

			ticket.FeeTxStatus = database.FeeError

			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error, failed to set fee tx error (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
			}

			sendErrorWithMsg("could not broadcast fee transaction", errCannotBroadcastFee, c)
			return
		}

		ticket.FeeTxStatus = database.FeeBroadcast

		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("%s: db.UpdateTicket error, failed to set fee tx as broadcast (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
			sendError(errInternalError, c)
			return
		}

		log.Debugf("%s: Fee tx broadcast for ticket (ticketHash=%s, feeHash=%s)",
			funcName, ticket.Hash, ticket.FeeTxHash)
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		Request:   request,
	}, c)
}
