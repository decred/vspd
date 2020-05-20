package webapi

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
	"github.com/jholdstock/dcrvsp/database"
)

// feeAddress is the handler for "POST /feeaddress"
func feeAddress(c *gin.Context) {
	var feeAddressRequest FeeAddressRequest
	if err := c.ShouldBindJSON(&feeAddressRequest); err != nil {
		log.Warnf("Bad feeaddress request from %s: %v", c.ClientIP(), err)
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := feeAddressRequest.TicketHash
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := feeAddressRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s: %v", c.ClientIP(), err)
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	// Check for existing response
	ticket, err := db.GetTicketByHash(ticketHashStr)
	if err != nil && !errors.Is(err, database.ErrNoTicketFound) {
		log.Errorf("GetTicketByHash error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}
	if err == nil {
		// Ticket already exists
		if signature == ticket.CommitmentSignature {
			now := time.Now()
			expire := ticket.Expiration
			VSPFee := ticket.VSPFee
			if now.After(time.Unix(ticket.Expiration, 0)) {
				expire = now.Add(cfg.FeeAddressExpiration).Unix()
				VSPFee = cfg.VSPFee

				err = db.UpdateExpireAndFee(ticketHashStr, expire, VSPFee)
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
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	walletClient, err := walletRPC()
	if err != nil {
		log.Errorf("Failed to dial dcrwallet RPC: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	ctx := c.Request.Context()

	var resp dcrdtypes.TxRawResult
	err = walletClient.Call(ctx, "getrawtransaction", &resp, txHash.String(), 1)
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

	// Get commitment address
	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, cfg.NetParams)
	if err != nil {
		log.Errorf("Failed to get commitment address: %v", err)
		sendErrorResponse("failed to get commitment address", http.StatusInternalServerError, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 getfeeaddress %s", msgTx.TxHash())
	err = dcrutil.VerifyMessage(addr.Address(), signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Invalid signature from %s: %v", c.ClientIP(), err)
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	// get blockheight and sdiff which is required by
	// txrules.StakePoolTicketFee, and store them in the database
	// for processing by payfee
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err = walletClient.Call(ctx, "getblockheader", &blockHeader, resp.BlockHash, true)
	if err != nil {
		log.Errorf("GetBlockHeader error: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	var newAddress string
	err = walletClient.Call(ctx, "getnewaddress", &newAddress, "fees")
	if err != nil {
		log.Errorf("GetNewAddress error: %v", err)
		sendErrorResponse("unable to generate fee address", http.StatusInternalServerError, c)
		return
	}

	now := time.Now()
	expire := now.Add(cfg.FeeAddressExpiration).Unix()

	dbTicket := database.Ticket{
		Hash:                txHash.String(),
		CommitmentSignature: signature,
		CommitmentAddress:   addr.Address(),
		FeeAddress:          newAddress,
		SDiff:               blockHeader.SBits,
		BlockHeight:         int64(blockHeader.Height),
		VSPFee:              cfg.VSPFee,
		Expiration:          expire,
		// VotingKey and VoteChoices: set during payfee
	}

	err = db.InsertFeeAddress(dbTicket)
	if err != nil {
		log.Errorf("InsertFeeAddress error: %v", err)
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
