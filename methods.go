package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
)

const (
	defaultFeeAddressExpiration = 24 * time.Hour
)

func sendJSONResponse(resp interface{}, code int, c *gin.Context) {
	dec, err := json.Marshal(resp)
	if err != nil {
		log.Printf("JSON marshal error: %v", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	sig := ed25519.Sign(cfg.signKey, dec)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.Header().Set("VSP-Signature", hex.EncodeToString(sig))
	c.Writer.WriteHeader(code)
	c.Writer.Write(dec)
}

func pubKey(c *gin.Context) {
	sendJSONResponse(pubKeyResponse{
		Timestamp: time.Now().Unix(),
		PubKey:    cfg.pubKey,
	}, http.StatusOK, c)
}

func fee(c *gin.Context) {
	sendJSONResponse(feeResponse{
		Timestamp: time.Now().Unix(),
		Fee:       cfg.VSPFee,
	}, http.StatusOK, c)
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

	// TODO: check db for cache response - if expired, reset expiration, but still
	// use same feeaddress

	ctx := c.Request.Context()

	var resp dcrdtypes.TxRawResult
	err = nodeConnection.Call(ctx, "getrawtransaction", &resp, txHash.String(), true)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, errors.New("unknown transaction"))
		return
	}
	if resp.Confirmations < 2 || resp.BlockHeight < 0 {
		c.AbortWithError(http.StatusBadRequest, errors.New("transaction does not have minimum confirmations"))
		return
	}
	if resp.Confirmations > int64(uint32(cfg.netParams.TicketMaturity)+cfg.netParams.TicketExpiry) {
		c.AbortWithError(http.StatusBadRequest, errors.New("ticket has expired"))
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
	addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, cfg.netParams)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("failed to get commitment address"))
		return
	}

	// verify message
	message := fmt.Sprintf("vsp v3 getfeeaddress %s", msgTx.TxHash())
	var valid bool
	err = nodeConnection.Call(ctx, "verifymessage", &valid, addr.Address(), signature, message)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}
	if !valid {
		c.AbortWithError(http.StatusBadRequest, errors.New("invalid signature"))
		return
	}

	// get blockheight and sdiff which is required by
	// txrules.StakePoolTicketFee, and store them in the database
	// for processing by payfee
	var blockHeader dcrdtypes.GetBlockHeaderVerboseResult
	err = nodeConnection.Call(ctx, "getblockheader", &blockHeader, resp.BlockHash, true)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}
	sDiff := blockHeader.SBits

	var newAddress string
	err = nodeConnection.Call(ctx, "getnewaddress", &newAddress, "fees")
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("unable to generate fee address"))
		return
	}

	// TODO: Insert into DB
	_ = sDiff

	now := time.Now()
	sendJSONResponse(feeAddressResponse{
		Timestamp:  now.Unix(),
		Request:    feeAddressRequest,
		FeeAddress: newAddress,
		Expiration: now.Add(defaultFeeAddressExpiration).Unix(),
	}, http.StatusOK, c)
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
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.netParams.PrivateKeyID)
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
		log.Fatalf("database error: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
		return
	}

	var feeAddr string
	var feeAmount dcrutil.Amount
	const scriptVersion = 0

findAddress:
	for _, txOut := range feeTx.TxOut {
		_, addresses, _, err := txscript.ExtractPkScriptAddrs(scriptVersion,
			txOut.PkScript, cfg.netParams)
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
	voteAddr, err := dcrutil.DecodeAddress(feeEntry.Address, cfg.netParams)
	if err != nil {
		fmt.Printf("PayFee: DecodeAddress: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
		return
	}
	_, err = dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.netParams,
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

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(feeEntry.BlockHeight), cfg.VSPFee, cfg.netParams.Params)
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
	}, http.StatusOK, c)
}

// PayFee2 is copied from the stakepoold implementation in #625
func PayFee2(ctx context.Context, ticketHash *chainhash.Hash, votingWIF *dcrutil.WIF, feeTx *wire.MsgTx) (string, error) {
	var resp dcrdtypes.TxRawResult
	err := nodeConnection.Call(ctx, "getrawtransaction", &resp, ticketHash.String(), true)
	if err != nil {
		fmt.Printf("PayFee: getrawtransaction: %v", err)
		return "", errors.New("RPC server error")
	}

	err = nodeConnection.Call(ctx, "addticket", nil, resp.Hex)
	if err != nil {
		fmt.Printf("PayFee: addticket: %v", err)
		return "", errors.New("RPC server error")
	}

	err = nodeConnection.Call(ctx, "importprivkey", nil, votingWIF.String(), "imported", false, 0)
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
	err = nodeConnection.Call(ctx, "sendrawtransaction", &res, hex.NewEncoder(feeTxBuf), false)
	if err != nil {
		fmt.Printf("PayFee: sendrawtransaction: %v", err)
		return "", errors.New("transaction failed to send")
	}
	return res, nil
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

	// TODO: DB - get commitment address taken during /feeaddress request
	// this will drop the need for getrawtransaction
	var addr string

	// verify message
	ctx := c.Request.Context()
	message := fmt.Sprintf("vsp v3 ticketstatus %d %s", ticketStatusRequest.Timestamp, ticketHashStr)
	var valid bool
	err = nodeConnection.Call(ctx, "verifymessage", &valid, addr, signature, message)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}
	if !valid {
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
	}, http.StatusOK, c)
}
