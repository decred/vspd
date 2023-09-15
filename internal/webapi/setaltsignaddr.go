// Copyright (c) 2021-2023 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"time"

	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/types/v2"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// Ensure that Node is satisfied by *rpc.DcrdRPC.
var _ node = (*rpc.DcrdRPC)(nil)

// node is satisfied by *rpc.DcrdRPC and retrieves data from the blockchain.
type node interface {
	ExistsLiveTicket(ticketHash string) (bool, error)
	GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error)
}

// setAltSignAddr is the handler for "POST /api/v3/setaltsignaddr".
func (w *WebAPI) setAltSignAddr(c *gin.Context) {

	const funcName = "setAltSignAddr"

	// Get values which have been added to context by middleware.
	dcrdClient := c.MustGet(dcrdKey).(node)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		w.log.Errorf("%s: Could not get dcrd client: %v", funcName, dcrdErr.(error))
		w.sendError(types.ErrInternalError, c)
		return
	}
	reqBytes := c.MustGet(requestBytesKey).([]byte)

	var request types.SetAltSignAddrRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		w.log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	altSignAddr, ticketHash := request.AltSignAddress, request.TicketHash

	currentData, err := w.db.AltSignAddrData(ticketHash)
	if err != nil {
		w.log.Errorf("%s: db.AltSignAddrData (ticketHash=%s): %v", funcName, ticketHash, err)
		w.sendError(types.ErrInternalError, c)
		return
	}
	if currentData != nil {
		msg := "alternate sign address data already exists"
		w.log.Warnf("%s: %s (ticketHash=%s)", funcName, msg, ticketHash)
		w.sendErrorWithMsg(msg, types.ErrBadRequest, c)
		return

	}

	// Fail fast if the pubkey doesn't decode properly.
	addr, err := stdaddr.DecodeAddressV0(altSignAddr, w.cfg.Network)
	if err != nil {
		w.log.Warnf("%s: Alt sign address cannot be decoded (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		w.log.Warnf("%s: Alt sign address is unexpected type (clientIP=%s, type=%T)", funcName, c.ClientIP(), addr)
		w.sendErrorWithMsg("wrong type for alternate signing address", types.ErrBadRequest, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticketHash)
	if err != nil {
		w.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v", funcName, ticketHash, err)
		w.sendError(types.ErrInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := canTicketVote(rawTicket, dcrdClient, w.cfg.Network)
	if err != nil {
		w.log.Errorf("%s: canTicketVote error (ticketHash=%s): %v", funcName, ticketHash, err)
		w.sendError(types.ErrInternalError, c)
		return
	}
	if !canVote {
		w.log.Warnf("%s: unvotable ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticketHash)
		w.sendError(types.ErrTicketCannotVote, c)
		return
	}

	// Send success response to client.
	resp, respSig := w.sendJSONResponse(types.SetAltSignAddrResponse{
		Timestamp: time.Now().Unix(),
		Request:   reqBytes,
	}, c)

	data := &database.AltSignAddrData{
		AltSignAddr: altSignAddr,
		Req:         string(reqBytes),
		ReqSig:      c.GetHeader("VSP-Client-Signature"),
		Resp:        resp,
		RespSig:     respSig,
	}

	err = w.db.InsertAltSignAddr(ticketHash, data)
	if err != nil {
		w.log.Errorf("%s: db.InsertAltSignAddr error (ticketHash=%s): %v",
			funcName, ticketHash, err)
		return
	}

	w.log.Debugf("%s: New alt sign address set for ticket: (ticketHash=%s)", funcName, ticketHash)
}
