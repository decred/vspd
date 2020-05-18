package webapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"decred.org/dcrwallet/wallet/txrules"
	"github.com/decred/dcrd/blockchain/stake/v3"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrutil/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v2"
	"github.com/decred/dcrd/txscript/v3"
	"github.com/decred/dcrd/wire"
	"github.com/gin-gonic/gin"
	"github.com/jholdstock/dcrvsp/database"
)

const (
	defaultFeeAddressExpiration = 24 * time.Hour
)

func sendJSONResponse(resp interface{}, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	sig := ed25519.Sign(cfg.SignKey, dec)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.Header().Set("VSP-Signature", hex.EncodeToString(sig))
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Write(dec)
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
	dec := json.NewDecoder(c.Request.Body)

	var feeAddressRequest FeeAddressRequest
	err := dec.Decode(&feeAddressRequest)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid json"))
		return
	}

	// ticketHash
	ticketHashStr := feeAddressRequest.TicketHash
	if len(ticketHashStr) != chainhash.MaxHashStringSize {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := feeAddressRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
		return
	}

	/*
		// TODO - DB - deal with cached responses
		ticket, err := db.GetFeeAddressByTicketHash(ticketHashStr)
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

	ctx := c.Request.Context()

	walletClient, err := walletRPC()
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("wallet RPC error"))
		return
	}

	var resp dcrdtypes.TxRawResult
	err = walletClient.Call(ctx, "getrawtransaction", &resp, txHash.String(), true)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("unknown transaction"))
		return
	}
	if resp.Confirmations < 2 || resp.BlockHeight < 0 {
		c.AbortWithError(http.StatusBadRequest, errors.New("transaction does not have minimum confirmations"))
		return
	}
	if resp.Confirmations > int64(uint32(cfg.NetParams.TicketMaturity)+cfg.NetParams.TicketExpiry) {
		c.AbortWithError(http.StatusBadRequest, errors.New("transaction too old"))
		return
	}

	msgHex, err := hex.DecodeString(resp.Hex)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("unable to decode transaction"))
		return
	}

	msgTx := wire.NewMsgTx()
	if err = msgTx.FromBytes(msgHex); err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("failed to deserialize transaction"))
		return
	}
	if !stake.IsSStx(msgTx) {
		c.AbortWithError(http.StatusBadRequest, errors.New("transaction is not a ticket"))
		return
	}
	if len(msgTx.TxOut) != 3 {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket"))
		return
	}

	// Get commitment address
	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, cfg.NetParams)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("failed to get commitment address"))
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 getfeeaddress %s", msgTx.TxHash())
	err = dcrutil.VerifyMessage(addr.Address(), signature, message, cfg.NetParams)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
		return
	}

	// get blockheight and sdiff which is required by
	// txrules.StakePoolTicketFee, and store them in the database
	// for processing by payfee
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err = walletClient.Call(ctx, "getblockheader", &blockHeader, resp.BlockHash, true)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}

	var newAddress string
	err = walletClient.Call(ctx, "getnewaddress", &newAddress, "fees")
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("unable to generate fee address"))
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

	// TODO: Insert into DB
	err = db.InsertFeeAddress(dbTicket)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
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
	dec := json.NewDecoder(c.Request.Body)

	var payFeeRequest PayFeeRequest
	err := dec.Decode(&payFeeRequest)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid json"))
		return
	}

	votingKey := payFeeRequest.VotingKey
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.NetParams.PrivateKeyID)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	voteBits := payFeeRequest.VoteBits

	feeTx := wire.NewMsgTx()
	err = feeTx.FromBytes(payFeeRequest.Hex)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("unable to deserialize transaction"))
		return
	}

	// TODO: DB - check expiration given during fee address request

	validFeeAddrs, err := db.GetInactiveFeeAddresses()
	if err != nil {
		log.Errorf("database error: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
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
			fmt.Printf("Extract: %v", err)
			c.AbortWithError(http.StatusInternalServerError, err)
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
		fmt.Printf("feeTx did not invalid any payments")
		c.AbortWithError(http.StatusInternalServerError, errors.New("feeTx did not include any payments"))
		return
	}

	feeEntry, err := db.GetFeesByFeeAddress(feeAddr)
	if err != nil {
		fmt.Printf("GetFeeByAddress: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
		return
	}
	voteAddr, err := dcrutil.DecodeAddress(feeEntry.CommitmentAddress, cfg.NetParams)
	if err != nil {
		fmt.Printf("PayFee: DecodeAddress: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
		return
	}
	_, err = dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.NetParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		fmt.Printf("PayFee: NewAddressPubKeyHash: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("failed to deserialize voting wif"))
		return
	}

	// TODO: DB - validate votingkey against ticket submission address

	sDiff := dcrutil.Amount(feeEntry.SDiff)

	// TODO - RPC - get relayfee from wallet
	relayFee, err := dcrutil.NewAmount(0.0001)
	if err != nil {
		fmt.Printf("PayFee: failed to NewAmount: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(feeEntry.BlockHeight), cfg.VSPFee, cfg.NetParams)
	if feeAmount < minFee {
		fmt.Printf("too cheap: %v %v", feeAmount, minFee)
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("dont get cheap on me, dodgson (sent:%v required:%v)", feeAmount, minFee))
		return
	}

	// Get vote tx to give to wallet
	ticketHash, err := chainhash.NewHashFromStr(feeEntry.Hash)
	if err != nil {
		fmt.Printf("PayFee: NewHashFromStr: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	now := time.Now()
	resp, err := PayFee2(c.Request.Context(), ticketHash, votingWIF, feeTx)
	if err != nil {
		fmt.Printf("PayFee: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}

	err = db.InsertFeeAddressVotingKey(voteAddr.Address(), votingWIF.String(), voteBits)
	if err != nil {
		fmt.Printf("PayFee: InsertVotingKey failed: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: now.Unix(),
		TxHash:    resp,
		Request:   payFeeRequest,
	}, c)
}

// PayFee2 is copied from the stakepoold implementation in #625
func PayFee2(ctx context.Context, ticketHash *chainhash.Hash, votingWIF *dcrutil.WIF, feeTx *wire.MsgTx) (string, error) {
	var resp dcrdtypes.TxRawResult

	walletClient, err := walletRPC()
	if err != nil {
		fmt.Printf("PayFee: wallet RPC error: %v", err)
		return "", errors.New("RPC server error")
	}

	err = walletClient.Call(ctx, "getrawtransaction", &resp, ticketHash.String(), true)
	if err != nil {
		fmt.Printf("PayFee: getrawtransaction: %v", err)
		return "", errors.New("RPC server error")
	}

	err = walletClient.Call(ctx, "addticket", nil, resp.Hex)
	if err != nil {
		fmt.Printf("PayFee: addticket: %v", err)
		return "", errors.New("RPC server error")
	}

	err = walletClient.Call(ctx, "importprivkey", nil, votingWIF.String(), "imported", false, 0)
	if err != nil {
		fmt.Printf("PayFee: importprivkey: %v", err)
		return "", errors.New("RPC server error")
	}

	feeTxBuf := new(bytes.Buffer)
	feeTxBuf.Grow(feeTx.SerializeSize())
	err = feeTx.Serialize(feeTxBuf)
	if err != nil {
		fmt.Printf("PayFee: failed to serialize fee transaction: %v", err)
		return "", errors.New("serialization error")
	}

	var res string
	err = walletClient.Call(ctx, "sendrawtransaction", &res, hex.NewEncoder(feeTxBuf), false)
	if err != nil {
		fmt.Printf("PayFee: sendrawtransaction: %v", err)
		return "", errors.New("transaction failed to send")
	}
	return res, nil
}

func setVoteBits(c *gin.Context) {
	dec := json.NewDecoder(c.Request.Body)

	var setVoteBitsRequest SetVoteBitsRequest
	err := dec.Decode(&setVoteBitsRequest)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid json"))
		return
	}

	// ticketHash
	ticketHashStr := setVoteBitsRequest.TicketHash
	if len(ticketHashStr) != chainhash.MaxHashStringSize {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}
	txHash, err := chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := setVoteBitsRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
		return
	}

	// votebits
	voteBits := setVoteBitsRequest.VoteBits
	if !isValidVoteBits(cfg.NetParams, currentVoteVersion(cfg.NetParams), voteBits) {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid votebits"))
		return
	}

	addr, err := db.GetCommitmentAddressByTicketHash(txHash.String())
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket"))
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 setvotebits %d %s %d", setVoteBitsRequest.Timestamp, txHash, voteBits)
	err = dcrutil.VerifyMessage(addr, signature, message, cfg.NetParams)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("message did not pass verification"))
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
	dec := json.NewDecoder(c.Request.Body)

	var ticketStatusRequest TicketStatusRequest
	err := dec.Decode(&ticketStatusRequest)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid json"))
		return
	}

	// ticketHash
	ticketHashStr := ticketStatusRequest.TicketHash
	if len(ticketHashStr) != chainhash.MaxHashStringSize {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}
	_, err = chainhash.NewHashFromStr(ticketHashStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket hash"))
		return
	}

	// signature - sanity check signature is in base64 encoding
	signature := ticketStatusRequest.Signature
	if _, err = base64.StdEncoding.DecodeString(signature); err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
		return
	}

	addr, err := db.GetCommitmentAddressByTicketHash(ticketHashStr)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid ticket"))
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 ticketstatus %d %s", ticketStatusRequest.Timestamp, ticketHashStr)
	err = dcrutil.VerifyMessage(addr, signature, message, cfg.NetParams)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
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
