package webapi

import (
	"time"

	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
)

// payFee is the handler for "POST /payfee".
func payFee(c *gin.Context) {
	funcName := "payFee"

	// Get values which have been added to context by middleware.
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	dcrdClient := c.MustGet("DcrdClient").(*rpc.DcrdRPC)

	if cfg.VspClosed {
		sendError(errVspClosed, c)
		return
	}

	if !knownTicket {
		log.Warnf("%s: Unknown ticket from %s", funcName, c.ClientIP())
		sendError(errUnknownTicket, c)
		return
	}

	var payFeeRequest PayFeeRequest
	if err := c.ShouldBindJSON(&payFeeRequest); err != nil {
		log.Warnf("%s: Bad request from %s: %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	// Respond early if we already have the fee tx for this ticket.
	if ticket.FeeTxStatus == database.FeeReceieved ||
		ticket.FeeTxStatus == database.FeeBroadcast ||
		ticket.FeeTxStatus == database.FeeConfirmed {
		log.Warnf("%s: Fee tx already received from %s: ticketHash=%s", funcName, c.ClientIP(), ticket.Hash)
		sendError(errFeeAlreadyReceived, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
	if err != nil {
		log.Errorf("%s: Could not retrieve tx %s for %s: %v", funcName, ticket.Hash, c.ClientIP(), err)
		sendError(errInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, ticket.Hash, cfg.NetParams)
	if err != nil {
		log.Errorf("%s: canTicketVote error: %v", funcName, err)
		sendError(errInternalError, c)
		return
	}
	if !canVote {
		log.Warnf("%s: Unvotable ticket %s from %s", funcName, ticket.Hash, c.ClientIP())
		sendError(errTicketCannotVote, c)
		return
	}

	// Respond early if the fee for this ticket is expired.
	if ticket.FeeExpired() {
		log.Warnf("%s: Expired payfee request from %s", funcName, c.ClientIP())
		sendError(errFeeExpired, c)
		return
	}

	// Validate VotingKey.
	votingKey := payFeeRequest.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		log.Warnf("%s: Failed to decode WIF: %v", funcName, err)
		sendError(errInvalidPrivKey, c)
		return
	}

	// Validate VoteChoices.
	voteChoices := payFeeRequest.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("%s: Invalid votechoices from %s: %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errInvalidVoteChoices, c)
		return
	}

	// Validate FeeTx.
	feeTx, err := decodeTransaction(payFeeRequest.FeeTx)
	if err != nil {
		log.Warnf("%s: Failed to decode tx: %v", funcName, err)
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
			sendErrorWithMsg("invalid script version", errInvalidFeeTx, c)
			return
		}
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(scriptVersion,
			txOut.PkScript, cfg.NetParams)
		if err != nil {
			log.Errorf("%s: Extract PK error: %v", funcName, err)
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
		log.Warnf("%s: FeeTx for ticket %s did not include any payments for address %s",
			funcName, ticket.Hash, ticket.FeeAddress)
		sendErrorWithMsg("feetx did not include any payments for fee address", errInvalidFeeTx, c)
		return
	}

	wifAddr, err := dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.NetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		log.Errorf("%s: NewAddressPubKeyHash: %v", funcName, err)
		sendError(errInvalidPrivKey, c)
		return
	}

	// Decode ticket transaction to get its voting address.
	ticketTx, err := decodeTransaction(rawTicket.Hex)
	if err != nil {
		log.Warnf("%s: Failed to decode tx: %v", funcName, err)
		sendError(errInternalError, c)
		return
	}

	// Get ticket voting address.
	_, votingAddr, _, err := txscript.ExtractPkScriptAddrs(scriptVersion, ticketTx.TxOut[0].PkScript, cfg.NetParams)
	if err != nil {
		log.Errorf("%s: ExtractPK error: %v", funcName, err)
		sendError(errInternalError, c)
		return
	}
	if len(votingAddr) == 0 {
		log.Error("%s: No voting address found for ticket %s", funcName, ticket.Hash)
		sendError(errInternalError, c)
		return
	}

	// Ensure provided private key will allow us to vote this ticket.
	if votingAddr[0].Address() != wifAddr.Address() {
		log.Warnf("%s: Voting address does not match provided private key: "+
			"votingAddr=%+v, wifAddr=%+v", funcName, votingAddr[0], wifAddr)
		sendErrorWithMsg("voting address does not match provided private key",
			errInvalidPrivKey, c)
		return
	}

	minFee := dcrutil.Amount(ticket.FeeAmount)
	if feePaid < minFee {
		log.Warnf("%s: Fee too small from %s: was %v, expected %v", funcName, c.ClientIP(),
			feePaid, minFee)
		sendError(errFeeTooSmall, c)
		return
	}

	// At this point we are satisfied that the request is valid and the FeeTx
	// pays sufficient fees to the expected address. Proceed to update the
	// database, and if the ticket is confirmed broadcast the transaction.

	ticket.VotingWIF = votingWIF.String()
	ticket.FeeTxHex = payFeeRequest.FeeTx
	ticket.FeeTxHash = feeTx.TxHash().String()
	ticket.VoteChoices = voteChoices
	ticket.FeeTxStatus = database.FeeReceieved

	err = db.UpdateTicket(ticket)
	if err != nil {
		log.Errorf("%s: InsertTicket failed: %v", funcName, err)
		sendError(errInternalError, c)
		return
	}

	log.Debugf("%s: Fee tx received for ticket: minExpectedFee=%v, feePaid=%v, "+
		"ticketHash=%s", funcName, minFee, feePaid, ticket.Hash)

	if ticket.Confirmed {
		err = dcrdClient.SendRawTransaction(payFeeRequest.FeeTx)
		if err != nil {
			log.Errorf("%s: SendRawTransaction failed: %v", funcName, err)

			ticket.FeeTxStatus = database.FeeError

			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: UpdateTicket error: %v", funcName, err)
			}

			sendErrorWithMsg("could not broadcast fee transaction", errInvalidFeeTx, c)
			return
		}

		ticket.FeeTxStatus = database.FeeBroadcast

		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("%s: UpdateTicket failed: %v", funcName, err)
			sendError(errInternalError, c)
			return
		}

		log.Debugf("%s: Fee tx broadcast for ticket: ticketHash=%s, feeHash=%s",
			funcName, ticket.Hash, ticket.FeeTxHash)
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		Request:   payFeeRequest,
	}, c)
}
