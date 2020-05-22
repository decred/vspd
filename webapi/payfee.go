package webapi

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"time"

	"decred.org/dcrwallet/wallet/txrules"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/dcrvsp/rpc"
)

// payFee is the handler for "POST /payfee".
func payFee(c *gin.Context) {

	ctx := c.Request.Context()

	reqBytes, err := c.GetRawData()
	if err != nil {
		log.Warnf("Error reading request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	var payFeeRequest PayFeeRequest
	if err := binding.JSON.BindBody(reqBytes, &payFeeRequest); err != nil {
		log.Warnf("Bad payfee request from %s: %v", c.ClientIP(), err)
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
	ticket, ticketFound, err := db.GetTicketByHash(payFeeRequest.TicketHash)
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
		commitmentAddress, err = fWalletClient.GetTicketCommitmentAddress(payFeeRequest.TicketHash, cfg.NetParams)
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

	// TODO: Respond early if the fee tx has already been broadcast for this
	// ticket. Maybe indicate status - mempool/awaiting confs/confirmed.

	if !ticketFound {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
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
	err = feeTx.FromBytes(feeTxBytes)
	if err != nil {
		log.Warnf("Failed to deserialize tx: %v", err)
		sendErrorResponse("unable to deserialize transaction", http.StatusBadRequest, c)
		return
	}

	feeTxBuf := new(bytes.Buffer)
	feeTxBuf.Grow(feeTx.SerializeSize())
	err = feeTx.Serialize(feeTxBuf)
	if err != nil {
		log.Errorf("Serialize tx failed: %v", err)
		sendErrorResponse("serialize tx error", http.StatusInternalServerError, c)
		return
	}

	if ticket.FeeExpired() {
		log.Warnf("Expired payfee request from %s", c.ClientIP())
		sendErrorResponse("fee has expired", http.StatusBadRequest, c)
		return
	}

	// Loop through transaction outputs until we find one which pays to the
	// expected fee address. Record how much is being paid to the fee address.
	var feeAmount dcrutil.Amount
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
				feeAmount = dcrutil.Amount(txOut.Value)
				break findAddress
			}
		}
	}

	if feeAmount == 0 {
		log.Warnf("FeeTx for ticket %s did not include any payments for address %s", ticket.Hash, ticket.FeeAddress)
		sendErrorResponse("feetx did not include any payments for fee address", http.StatusBadRequest, c)
		return
	}

	_, err = dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.NetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		log.Errorf("NewAddressPubKeyHash: %v", err)
		sendErrorResponse("failed to deserialize voting wif", http.StatusInternalServerError, c)
		return
	}

	// TODO: DB - validate votingkey against ticket submission address

	sDiff := dcrutil.Amount(ticket.SDiff)

	relayFee, err := fWalletClient.GetWalletFee()
	if err != nil {
		log.Errorf("GetWalletFee failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(ticket.BlockHeight), cfg.VSPFee, cfg.NetParams)
	if feeAmount < minFee {
		log.Errorf("Fee too small: was %v, expected %v", feeAmount, minFee)
		sendErrorResponse("fee too small", http.StatusInternalServerError, c)
		return
	}

	// At this point we are satisfied that the request is valid and the FeeTx
	// pays sufficient fees to the expected address.
	// Proceed to update the database and broadcast the transaction.

	feeTxHash, err := fWalletClient.SendRawTransaction(hex.EncodeToString(feeTxBuf.Bytes()))
	if err != nil {
		log.Errorf("SendRawTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = db.SetTicketVotingKey(ticket.Hash, votingWIF.String(), voteChoices, feeTxHash)
	if err != nil {
		log.Errorf("SetTicketVotingKey failed: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	// TODO: Should return a response here. We don't want to add the ticket to
	// the voting wallets until the fee tx has been confirmed.

	// Add ticket to voting wallets.
	rawTicket, err := fWalletClient.GetRawTransaction(ticket.Hash)
	if err != nil {
		log.Warnf("Could not retrieve tx %s for %s: %v", ticket.Hash, c.ClientIP(), err)
		sendErrorResponse("unknown transaction", http.StatusBadRequest, c)
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

	err = vWalletClient.AddTransaction(rawTicket.BlockHash, rawTicket.Hex)
	if err != nil {
		log.Errorf("AddTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = vWalletClient.ImportPrivKey(votingWIF.String())
	if err != nil {
		log.Errorf("ImportPrivKey failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
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

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		TxHash:    feeTxHash,
		Request:   payFeeRequest,
	}, c)
}
