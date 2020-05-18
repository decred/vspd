package webapi

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jholdstock/dcrvsp/database"

	"decred.org/dcrwallet/wallet/txrules"
	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
)

const (
	defaultFeeAddressExpiration = 24 * time.Hour
)

func sendJSONResponse(resp interface{}, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
		sendErrorResponse("failed to marshal json", http.StatusInternalServerError, c)
		return
	}

	sig := ed25519.Sign(cfg.SignKey, dec)
	c.Writer.Header().Set("VSP-Signature", hex.EncodeToString(sig))

	c.JSON(http.StatusOK, resp)
}

func sendErrorResponse(errMsg string, code int, c *gin.Context) {
	c.JSON(code, gin.H{"error": errMsg})
}

func pubKey(c *gin.Context) {
	sendJSONResponse(pubKeyResponse{
		Timestamp: time.Now().Unix(),
		PubKey:    cfg.PubKey,
	}, c)
}

func fee(c *gin.Context) {
	sendJSONResponse(feeResponse{
		Timestamp: time.Now().Unix(),
		Fee:       cfg.VSPFee,
	}, c)
}

func feeAddress(c *gin.Context) {

	var feeAddressRequest FeeAddressRequest
	if err := c.ShouldBindJSON(&feeAddressRequest); err != nil {
		log.Warnf("Bad request from %s", c.ClientIP())
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
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	/*
		// TODO - DB - deal with cached responses
		ticket, err := db.GetTicketByHash(ticketHashStr)
		if err != nil && !errors.Is(err, database.ErrNoTicketFound) {
			c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
			return
		}
		if err == nil {
			// TODO - deal with expiration
			if signature == ticket.CommitmentSignature {
				sendJSONResponse(feeAddressResponse{
					Timestamp:           time.Now().Unix(),
					CommitmentSignature: ticket.CommitmentSignature,
					FeeAddress:          ticket.FeeAddress,
					Expiration: 	ticket.Expiration,
					}, http.StatusOK, c)
				return
			}
			c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
			return
		}
	*/

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
		log.Warnf("Could not retrieve tx for %s", c.ClientIP())
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
		log.Warnf("Invalid signature from %s", c.ClientIP())
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
	expire := now.Add(defaultFeeAddressExpiration).Unix()

	dbTicket := database.Ticket{
		Hash:                txHash.String(),
		CommitmentSignature: signature,
		CommitmentAddress:   addr.Address(),
		FeeAddress:          newAddress,
		SDiff:               blockHeader.SBits,
		BlockHeight:         int64(blockHeader.Height),
		VoteBits:            dcrutil.BlockValid,
		Expiration:          expire,
		// VotingKey: set during payfee
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
		Expiration: expire,
	}, c)
}

func payFee(c *gin.Context) {
	var payFeeRequest PayFeeRequest
	if err := c.ShouldBindJSON(&payFeeRequest); err != nil {
		log.Warnf("Bad request from %s", c.ClientIP())
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	votingKey := payFeeRequest.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		log.Errorf("Failed to decode WIF: %v", err)
		sendErrorResponse("error decoding WIF", http.StatusInternalServerError, c)
		return
	}

	voteBits := payFeeRequest.VoteBits

	feeTx := wire.NewMsgTx()
	err = feeTx.FromBytes(payFeeRequest.Hex)
	if err != nil {
		log.Errorf("Failed to deserialize tx: %v", err)
		sendErrorResponse("unable to deserialize transaction", http.StatusInternalServerError, c)
		return
	}

	// TODO: DB - check expiration given during fee address request

	validFeeAddrs, err := db.GetInactiveFeeAddresses()
	if err != nil {
		log.Errorf("GetInactiveFeeAddresses error: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	var feeAddr string
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
			addrStr := addr.Address()
			for _, validFeeAddr := range validFeeAddrs {
				if addrStr == validFeeAddr {
					feeAddr = validFeeAddr
					feeAmount = dcrutil.Amount(txOut.Value)
					break findAddress
				}
			}
		}
	}
	if feeAddr == "" {
		log.Errorf("feeTx did not include any payments")
		sendErrorResponse("feeTx did not include any payments", http.StatusInternalServerError, c)
		return
	}

	feeEntry, err := db.GetFeesByFeeAddress(feeAddr)
	if err != nil {
		log.Errorf("GetFeeByAddress: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}
	voteAddr, err := dcrutil.DecodeAddress(feeEntry.CommitmentAddress, cfg.NetParams)
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

	sDiff := dcrutil.Amount(feeEntry.SDiff)

	// TODO - RPC - get relayfee from wallet
	relayFee, err := dcrutil.NewAmount(0.0001)
	if err != nil {
		log.Errorf("NewAmount failed: %v", err)
		sendErrorResponse("failed to create new amount", http.StatusInternalServerError, c)
		return
	}

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(feeEntry.BlockHeight), cfg.VSPFee, cfg.NetParams)
	if feeAmount < minFee {
		log.Errorf("Fee too small: was %v, expected %v", feeAmount, minFee)
		sendErrorResponse("fee too small", http.StatusInternalServerError, c)
		return
	}

	// Get vote tx to give to wallet
	ticketHash, err := chainhash.NewHashFromStr(feeEntry.Hash)
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
	var resp dcrdtypes.TxRawResult

	err = walletClient.Call(ctx, "getrawtransaction", &resp, ticketHash.String(), true)
	if err != nil {
		log.Errorf("GetRawTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = walletClient.Call(ctx, "addticket", nil, resp.Hex)
	if err != nil {
		log.Errorf("AddTicket failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = walletClient.Call(ctx, "importprivkey", nil, votingWIF.String(), "imported", false, 0)
	if err != nil {
		log.Errorf("ImportPrivKey failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
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

	var res string
	err = walletClient.Call(ctx, "sendrawtransaction", &res, hex.NewEncoder(feeTxBuf), false)
	if err != nil {
		log.Errorf("SendRawTransaction failed: %v", err)
		sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
		return
	}

	err = db.InsertFeeAddressVotingKey(voteAddr.Address(), votingWIF.String(), voteBits)
	if err != nil {
		log.Errorf("InsertFeeAddressVotingKey failed: %v", err)
		sendErrorResponse("database error", http.StatusInternalServerError, c)
		return
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: time.Now().Unix(),
		TxHash:    res,
		Request:   payFeeRequest,
	}, c)
}

func setVoteBits(c *gin.Context) {
	var setVoteBitsRequest SetVoteBitsRequest
	if err := c.ShouldBindJSON(&setVoteBitsRequest); err != nil {
		log.Warnf("Bad request from %s", c.ClientIP())
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := setVoteBitsRequest.TicketHash
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := setVoteBitsRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	// votebits
	voteBits := setVoteBitsRequest.VoteBits
	if !isValidVoteBits(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteBits) {
		log.Warnf("Invalid votebits from %s", c.ClientIP())
		sendErrorResponse("invalid votebits", http.StatusBadRequest, c)
		return
	}

	ticket, err := db.GetTicketByHash(txHash.String())
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 setvotebits %d %s %d", setVoteBitsRequest.Timestamp, txHash, voteBits)
	err = dcrutil.VerifyMessage(ticket.CommitmentAddress, signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Failed to verify message from %s", c.ClientIP())
		sendErrorResponse("message did not pass verification", http.StatusBadRequest, c)
		return
	}

	// TODO: DB - error if given timestamp is older than any previous requests

	// TODO: DB - store setvotebits receipt in log

	sendJSONResponse(setVoteBitsResponse{
		Timestamp: time.Now().Unix(),
		Request:   setVoteBitsRequest,
		VoteBits:  voteBits,
	}, c)
}

func ticketStatus(c *gin.Context) {
	var ticketStatusRequest TicketStatusRequest
	if err := c.ShouldBindJSON(&ticketStatusRequest); err != nil {
		log.Warnf("Bad request from %s", c.ClientIP())
		sendErrorResponse(err.Error(), http.StatusBadRequest, c)
		return
	}

	// ticketHash
	ticketHashStr := ticketStatusRequest.TicketHash
	_, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket hash from %s", c.ClientIP())
		sendErrorResponse("invalid ticket hash", http.StatusBadRequest, c)
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := ticketStatusRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	ticket, err := db.GetTicketByHash(ticketHashStr)
	if err != nil {
		log.Warnf("Invalid ticket from %s", c.ClientIP())
		sendErrorResponse("invalid ticket", http.StatusBadRequest, c)
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 ticketstatus %d %s", ticketStatusRequest.Timestamp, ticketHashStr)
	err = dcrutil.VerifyMessage(ticket.CommitmentAddress, signature, message, cfg.NetParams)
	if err != nil {
		log.Warnf("Invalid signature from %s", c.ClientIP())
		sendErrorResponse("invalid signature", http.StatusBadRequest, c)
		return
	}

	// TODO: DB - get current votebits, get ticket status
	var voteBits uint16

	sendJSONResponse(ticketStatusResponse{
		Timestamp: time.Now().Unix(),
		Request:   ticketStatusRequest,
		Status:    "active", // TODO - active, pending, expired (missed, revoked?)
		VoteBits:  voteBits,
	}, c)
}
