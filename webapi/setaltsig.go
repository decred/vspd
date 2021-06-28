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

// setAltSig is the handler for "POST /api/v3/setaltsig".
func setAltSig(c *gin.Context) {

	const funcName = "setAltSig"

	// Get values which have been added to context by middleware.
	dcrdClient := c.MustGet("DcrdClient").(Node)
	reqBytes := c.MustGet("RequestBytes").([]byte)

	if cfg.VspClosed {
		sendError(errVspClosed, c)
		return
	}

	var request SetAltSigRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	altSigAddr, ticketHash := request.AltSigAddress, request.TicketHash

	currentData, err := db.AltSigData(ticketHash)
	if err != nil {
		log.Errorf("%s: db.AltSigData (ticketHash=%s): %v", funcName, ticketHash, err)
		sendError(errInternalError, c)
		return
	}
	if currentData != nil {
		msg := "alternate signature data already exists"
		log.Warnf("%s: %s (ticketHash=%s)", funcName, msg, ticketHash)
		sendErrorWithMsg(msg, errBadRequest, c)
		return

	}

	// Fail fast if the pubkey doesn't decode properly.
	addr, err := stdaddr.DecodeAddressV0(altSigAddr, cfg.NetParams)
	if err != nil {
		log.Warnf("%s: Alt sig address cannot be decoded (clientIP=%s): %v", funcName, c.ClientIP(), err)
		sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		log.Warnf("%s: Alt sig address is unexpected type (clientIP=%s, type=%T)", funcName, c.ClientIP(), addr)
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

	sigStr := c.GetHeader("VSP-Client-Signature")

	// Send success response to client.
	res, resSig := sendJSONResponse(SetAltSigResponse{
		Timestamp: time.Now().Unix(),
		Request:   reqBytes,
	}, c)

	data := &database.AltSigData{
		AltSigAddr: altSigAddr,
		Req:        reqBytes,
		ReqSig:     sigStr,
		Res:        []byte(res),
		ResSig:     resSig,
	}

	err = db.InsertAltSig(ticketHash, data)
	if err != nil {
		log.Errorf("%s: db.InsertAltSig error, failed to set alt data (ticketHash=%s): %v",
			funcName, ticketHash, err)
		return
	}

	log.Debugf("%s: New alt sig pubkey set for ticket: (ticketHash=%s)", funcName, ticketHash)
}
