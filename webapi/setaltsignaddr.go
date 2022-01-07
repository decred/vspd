// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// Ensure that Node is satisfied by *rpc.DcrdRPC.
var _ Node = (*rpc.DcrdRPC)(nil)

// Node is satisfied by *rpc.DcrdRPC and retrieves data from the blockchain.
type Node interface {
	CanTicketVote(rawTx *dcrdtypes.TxRawResult, ticketHash string, netParams *chaincfg.Params) (bool, error)
	GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error)
}

// setAltSignAddr is the handler for "POST /api/v3/setaltsignaddr".
func setAltSignAddr(c *gin.Context) {

	const funcName = "setAltSignAddr"

	// Get values which have been added to context by middleware.
	dcrdClient := c.MustGet(dcrdKey).(Node)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		log.Errorf("%s: could not get dcrd client: %v", funcName, dcrdErr.(error))
		sendError(errInternalError, c)
		return
	}
	reqBytes := c.MustGet(requestBytesKey).([]byte)

	if cfg.VspClosed {
		sendError(errVspClosed, c)
		return
	}

	var request setAltSignAddrRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	altSignAddr, ticketHash := request.AltSignAddress, request.TicketHash

	currentData, err := db.AltSignAddrData(ticketHash)
	if err != nil {
		log.Errorf("%s: db.AltSignAddrData (ticketHash=%s): %v", funcName, ticketHash, err)
		sendError(errInternalError, c)
		return
	}
	if currentData != nil {
		msg := "alternate sign address data already exists"
		log.Warnf("%s: %s (ticketHash=%s)", funcName, msg, ticketHash)
		sendErrorWithMsg(msg, errBadRequest, c)
		return

	}

	// Fail fast if the pubkey doesn't decode properly.
	addr, err := stdaddr.DecodeAddressV0(altSignAddr, cfg.NetParams)
	if err != nil {
		log.Warnf("%s: Alt sign address cannot be decoded (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		log.Warnf("%s: Alt sign address is unexpected type (clientIP=%s, type=%T)", funcName, c.ClientIP(), addr)
		sendErrorWithMsg("wrong type for alternate signing address", errBadRequest, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticketHash)
	if err != nil {
		log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v", funcName, ticketHash, err)
		sendError(errInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, ticketHash, cfg.NetParams)
	if err != nil {
		log.Errorf("%s: dcrd.CanTicketVote error (ticketHash=%s): %v", funcName, ticketHash, err)
		sendError(errInternalError, c)
		return
	}
	if !canVote {
		log.Warnf("%s: unvotable ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticketHash)
		sendError(errTicketCannotVote, c)
		return
	}

	// Prepare response to client.
	resp, respSig := prepareJSONResponse(setAltSignAddrResponse{
		Timestamp: time.Now().Unix(),
		Request:   reqBytes,
	}, c)

	data := &database.AltSignAddrData{
		AltSignAddr: altSignAddr,
		Req:         reqBytes,
		ReqSig:      c.GetHeader("VSP-Client-Signature"),
		Resp:        []byte(resp),
		RespSig:     respSig,
	}

	err = db.InsertAltSignAddr(ticketHash, data)
	if err != nil {
		log.Errorf("%s: db.InsertAltSignAddr error (ticketHash=%s): %v", funcName, ticketHash, err)
		sendError(errInternalError, c)
		return
	}

	// Send success response to client.
	sendJSONSuccess(resp, respSig, c)
	log.Debugf("%s: New alt sign address set for ticket: (ticketHash=%s)", funcName, ticketHash)
}
