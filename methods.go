package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	c.Writer.Write(dec)
}

func payFee(c *gin.Context) {
	// HTTP GET Params required
	// feeTx - serialized wire.MsgTx
	// votingKey - WIF private key for ticket stakesubmission address
	// voteBits - voting preferences in little endian

	votingKey := c.Param("votingKey")
	votingWIF, err := dcrutil.DecodeWIF(votingKey, cfg.netParams.PrivateKeyID)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	feeTxStr := c.Param("feeTx")
	feeTxBytes, err := hex.DecodeString(feeTxStr)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("invalid transaction"))
		return
	}

	voteBitsStr := c.Param("voteBits")
	voteBitsBytes, err := hex.DecodeString(voteBitsStr)
	if err != nil || len(voteBitsBytes) != 2 {
		c.AbortWithError(http.StatusInternalServerError, errors.New("invalid votebits"))
		return
	}

	voteBits := binary.LittleEndian.Uint16(voteBitsBytes)

	feeTx := wire.NewMsgTx()
	err = feeTx.FromBytes(feeTxBytes)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, errors.New("unable to deserialize transaction"))
		return
	}

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
		fmt.Errorf("PayFee: DecodeAddress: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("database error"))
		return
	}
	_, err = dcrutil.NewAddressPubKeyHash(dcrutil.Hash160(votingWIF.PubKey()), cfg.netParams,
		dcrec.STEcdsaSecp256k1)
	if err != nil {
		fmt.Errorf("PayFee: NewAddressPubKeyHash: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("failed to deserialize voting wif"))
		return
	}
	// TODO: validate votingkey against ticket submission address

	sDiff := dcrutil.Amount(feeEntry.SDiff)
	// TODO - wallet relayfee
	relayFee, err := dcrutil.NewAmount(0.0001)
	if err != nil {
		fmt.Errorf("PayFee: failed to NewAmount: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	minFee := txrules.StakePoolTicketFee(sDiff, relayFee, int32(feeEntry.BlockHeight), cfg.poolFees, cfg.netParams)
	if feeAmount < minFee {
		fmt.Printf("too cheap: %v %v", feeAmount, minFee)
		c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("dont get cheap on me, dodgson (sent:%v required:%v)", feeAmount, minFee))
		return
	}

	// Get vote tx to give to wallet
	ticketHash, err := chainhash.NewHashFromStr(feeEntry.Hash)
	if err != nil {
		fmt.Errorf("PayFee: NewHashFromStr: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	now := time.Now()
	resp, err := PayFee2(c.Request.Context(), ticketHash, votingWIF, feeTx)
	if err != nil {
		fmt.Errorf("PayFee: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("RPC server error"))
		return
	}

	err = db.InsertFeeAddressVotingKey(voteAddr.Address(), votingWIF.String(), voteBits)
	if err != nil {
		fmt.Errorf("PayFee: InsertVotingKey failed: %v", err)
		c.AbortWithError(http.StatusInternalServerError, errors.New("internal error"))
		return
	}

	sendJSONResponse(payFeeResponse{
		Timestamp: now.Unix(),
		TxHash:    resp,
	}, http.StatusOK, c)
}

// PayFee2 is copied from the stakepoold implementation in #625
func PayFee2(ctx context.Context, ticketHash *chainhash.Hash, votingWIF *dcrutil.WIF, feeTx *wire.MsgTx) (string, error) {
	var resp dcrdtypes.TxRawResult
	err := nodeConnection.Call(ctx, "getrawtransaction", &resp, ticketHash.String())
	if err != nil {
		fmt.Errorf("PayFee: getrawtransaction: %v", err)
		return "", errors.New("RPC server error")
	}

	err = nodeConnection.Call(ctx, "addticket", nil, resp.Hex)
	if err != nil {
		fmt.Errorf("PayFee: addticket: %v", err)
		return "", errors.New("RPC server error")
	}

	err = nodeConnection.Call(ctx, "importprivkey", nil, votingWIF.String(), "imported", false, 0)
	if err != nil {
		fmt.Errorf("PayFee: importprivkey: %v", err)
		return "", errors.New("RPC server error")
	}

	feeTxBuf := new(bytes.Buffer)
	feeTxBuf.Grow(feeTx.SerializeSize())
	err = feeTx.Serialize(feeTxBuf)
	if err != nil {
		fmt.Errorf("PayFee: failed to serialize fee transaction: %v", err)
		return "", errors.New("serialization error")
	}

	var res string
	err = nodeConnection.Call(ctx, "sendrawtransaction", &res, hex.NewEncoder(feeTxBuf), false)
	if err != nil {
		fmt.Errorf("PayFee: sendrawtransaction: %v", err)
		return "", errors.New("transaction failed to send")
	}
	return res, nil
}
