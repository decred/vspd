package webapi

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"time"

	"decred.org/dcrwallet/wallet/txrules"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
)

// payFee is the handler for "POST /payfee"
func payFee(c *gin.Context) {
	var payFeeRequest PayFeeRequest
	if err := c.ShouldBindJSON(&payFeeRequest); err != nil {
		log.Warnf("Bad payfee request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	votingKey := payFeeRequest.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		log.Warnf("Failed to decode WIF: %v", err)
		sendErrorResponse("error decoding WIF", http.StatusBadRequest, c)
		return
	}

	voteChoices := payFeeRequest.VoteChoices
	err = isValidVoteChoices(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteChoices)
	if err != nil {
		log.Warnf("Invalid votechoices from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

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

	// TODO: DB - check expiration given during fee address request

	ticket, err := db.GetTicketByHash(payFeeRequest.TicketHash)
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}
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

	voteAddr, err := dcrutil.DecodeAddress(ticket.CommitmentAddress, cfg.NetParams)
	if err != nil {
		log.Errorf("DecodeAddress: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
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

	// TODO - RPC - get relayfee from wallet
	relayFee, err := dcrutil.NewAmount(0.0001)
	if err != nil {
		log.Errorf("NewAmount failed: %v", err)
		sendErrorResponse("failed to create new amount", http.StatusInternalServerError, c)
		return
	}

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(ticket.BlockHeight), cfg.VSPFee, cfg.NetParams)
	if feeAmount < minFee {
		log.Errorf("Fee too small: was %v, expected %v", feeAmount, minFee)
		sendErrorResponse("fee too small", http.StatusInternalServerError, c)
		return
	}

	// Get vote tx to give to wallet
	ticketHash, err := chainhash.NewHashFromStr(ticket.Hash)
	if err != nil {
		log.Errorf("NewHashFromStr failed: %v", err)
		sendErrorResponse("failed to create hash", http.StatusInternalServerError, c)
		return
	}

	walletClient, err := walletRPC()
	if err != nil {
		log.Errorf("Failed to dial dcrwallet RPC: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	ctx := c.Request.Context()
	var rawTicket dcrdtypes.TxRawResult

	err = walletClient.Call(ctx, "getrawtransaction", &rawTicket, ticketHash.String(), 1)
	if err != nil {
		log.Errorf("GetRawTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = walletClient.Call(ctx, "addtransaction", nil, rawTicket.BlockHash, rawTicket.Hex)
	if err != nil {
		log.Errorf("AddTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = walletClient.Call(ctx, "importprivkey", nil, votingWIF.String(), "imported", false, 0)
	if err != nil {
		log.Errorf("ImportPrivKey failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	// Update vote choices on voting wallets.
	for agenda, choice := range voteChoices {
		err = walletClient.Call(ctx, "setvotechoice", nil, agenda, choice, ticket.Hash)
		if err != nil {
			log.Errorf("setvotechoice failed: %v", err)
			sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
			return
		}
	}

	feeTxBuf := new(bytes.Buffer)
	feeTxBuf.Grow(feeTx.SerializeSize())
	err = feeTx.Serialize(feeTxBuf)
	if err != nil {
		log.Errorf("Serialize tx failed: %v", err)
		sendErrorResponse("serialize tx error", http.StatusInternalServerError, c)
		return
	}

	var sendTxHash string
	err = walletClient.Call(ctx, "sendrawtransaction", &sendTxHash, hex.EncodeToString(feeTxBuf.Bytes()), false)
	if err != nil {
		log.Errorf("SendRawTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = db.InsertFeeAddressVotingKey(voteAddr.Address(), votingWIF.String(), voteChoices)
	if err != nil {
		log.Errorf("InsertFeeAddressVotingKey failed: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		TxHash:    sendTxHash,
		Request:   payFeeRequest,
	}, c)
}
