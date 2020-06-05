package webapi

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// payFee is the handler for "POST /payfee".
func payFee(c *gin.Context) {

	// Get values which have been added to context by middleware.
	rawRequest := c.MustGet("RawRequest").([]byte)
	ticket := c.MustGet("Ticket").(database.Ticket)
	knownTicket := c.MustGet("KnownTicket").(bool)
	dcrdClient := c.MustGet("DcrdClient").(*rpc.DcrdRPC)

	if !knownTicket {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	var payFeeRequest PayFeeRequest
	if err := binding.JSON.BindBody(rawRequest, &payFeeRequest); err != nil {
		log.Warnf("Bad payfee request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// Respond early if we already have the fee tx for this ticket.
	if ticket.FeeTxHex != "" {
		log.Warnf("Fee tx already received from %s: ticketHash=%s", c.ClientIP(), ticket.Hash)
		sendErrorResponse("fee tx already received", http.StatusBadRequest, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
	if err != nil {
		log.Errorf("Could not retrieve tx %s for %s: %v", ticket.Hash, c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusInternalServerError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, ticket.Hash, cfg.NetParams)
	if err != nil {
		log.Errorf("canTicketVote error: %v", err)
		sendErrorResponse("error validating ticket", http.StatusInternalServerError, c)
		return
	}
	if !canVote {
		log.Warnf("Unvotable ticket %s from %s", ticket.Hash, c.ClientIP())
		sendErrorResponse("ticket not eligible to vote", http.StatusBadRequest, c)
		return
	}

	// Respond early if the fee for this ticket is expired.
	if ticket.FeeExpired() {
		log.Warnf("Expired payfee request from %s", c.ClientIP())
		sendErrorResponse("fee has expired", http.StatusBadRequest, c)
		return
	}

	// Validate VotingKey.
	votingKey := payFeeRequest.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		log.Warnf("Failed to decode WIF: %v", err)
		sendErrorResponse("error decoding WIF", http.StatusBadRequest, c)
		return
	}

	// Validate VoteChoices.
	voteChoices := payFeeRequest.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("Invalid votechoices from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// Validate FeeTx.
	feeTxBytes, err := hex.DecodeString(payFeeRequest.FeeTx)
	if err != nil {
		log.Warnf("Failed to decode tx: %v", err)
		sendErrorResponse("failed to decode transaction", http.StatusBadRequest, c)
		return
	}

	feeTx := wire.NewMsgTx()
	if err = feeTx.FromBytes(feeTxBytes); err != nil {
		log.Warnf("Failed to deserialize tx: %v", err)
		sendErrorResponse("unable to deserialize transaction", http.StatusBadRequest, c)
		return
	}

	// Loop through transaction outputs until we find one which pays to the
	// expected fee address. Record how much is being paid to the fee address.
	var feePaid dcrutil.Amount
	const scriptVersion = 0

findAddress:
	for _, txOut := range feeTx.TxOut {
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(scriptVersion,
			txOut.PkScript, cfg.NetParams)
		if err != nil {
			log.Errorf("Extract PK error: %v", err)
			sendErrorResponse("extract PK error", http.StatusInternalServerError, c)
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
		log.Warnf("FeeTx for ticket %s did not include any payments for address %s", ticket.Hash, ticket.FeeAddress)
		sendErrorResponse("feetx did not include any payments for fee address", http.StatusBadRequest, c)
		return
	}

	wifAddr, err := dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.NetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		log.Errorf("NewAddressPubKeyHash: %v", err)
		sendErrorResponse("failed to deserialize voting wif", http.StatusInternalServerError, c)
		return
	}

	// Decode ticket transaction to get its voting address.
	ticketBytes, err := hex.DecodeString(rawTicket.Hex)
	if err != nil {
		log.Warnf("Failed to decode tx: %v", err)
		sendErrorResponse("failed to decode transaction", http.StatusBadRequest, c)
		return
	}
	ticketTx := wire.NewMsgTx()
	if err = ticketTx.FromBytes(ticketBytes); err != nil {
		log.Errorf("Failed to deserialize tx: %v", err)
		sendErrorResponse("unable to deserialize transaction", http.StatusInternalServerError, c)
		return
	}

	// Get ticket voting address.
	_, votingAddr, _, err := txscript.ExtractPkScriptAddrs(scriptVersion, ticketTx.TxOut[0].PkScript, cfg.NetParams)
	if err != nil {
		log.Errorf("ExtractPK error: %v", err)
		sendErrorResponse("extract PK error", http.StatusInternalServerError, c)
		return
	}
	if len(votingAddr) == 0 {
		log.Error("No voting address found for ticket %s", ticket.Hash)
		sendErrorResponse("no voting address found", http.StatusInternalServerError, c)
		return
	}

	// Ensure provided private key will allow us to vote this ticket.
	if votingAddr[0].Address() != wifAddr.Address() {
		log.Warnf("Voting address does not match provided private key: "+
			"votingAddr=%+v, wifAddr=%+v", votingAddr[0], wifAddr)
		sendErrorResponse("voting address does not match provided private key",
			http.StatusBadRequest, c)
		return
	}

	minFee, err := dcrutil.NewAmount(ticket.FeeAmount)
	if err != nil {
		log.Errorf("dcrutil.NewAmount: %v", err)
		sendErrorResponse("fee error", http.StatusInternalServerError, c)
		return
	}

	if feePaid < minFee {
		log.Warnf("Fee too small from %s: was %v, expected %v", c.ClientIP(), feePaid, minFee)
		sendErrorResponse("fee too small", http.StatusInternalServerError, c)
		return
	}

	// At this point we are satisfied that the request is valid and the FeeTx
	// pays sufficient fees to the expected address. Proceed to update the
	// database, and if the ticket is confirmed broadcast the transaction.

	ticket.VotingWIF = votingWIF.String()
	ticket.FeeTxHex = payFeeRequest.FeeTx
	ticket.VoteChoices = voteChoices

	err = db.UpdateTicket(ticket)
	if err != nil {
		log.Errorf("InsertTicket failed: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	log.Debugf("Fee tx received for ticket: minExpectedFee=%v, feePaid=%v, "+
		"ticketHash=%s", minFee, feePaid, ticket.Hash)

	if ticket.Confirmed {
		feeTxHash, err := dcrdClient.SendRawTransaction(payFeeRequest.FeeTx)
		if err != nil {
			// TODO: SendRawTransaction can return a "transcation already
			// exists" error, which isnt necessarily a problem here.
			log.Errorf("SendRawTransaction failed: %v", err)
			sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
			return
		}
		ticket.FeeTxHash = feeTxHash

		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("InsertTicket failed: %v", err)
			sendErrorResponse("database error", http.StatusInternalServerError, c)
			return
		}

		log.Debugf("Fee tx broadcast for ticket: ticketHash=%s, feeHash=%s", ticket.Hash, feeTxHash)
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		Request:   payFeeRequest,
	}, c)
}
