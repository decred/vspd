// Copyright (c) 2021-2022 The Decred developers
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
	CanTicketVote(rawTx *dcrdtypes.TxRawResult, netParams *chaincfg.Params) (bool, error)
	GetRawTransaction(txHash string) (*dcrdtypes.TxRawResult, error)
}

// setAltSignAddr is the handler for "POST /api/v3/setaltsignaddr".
func (s *Server) setAltSignAddr(c *gin.Context) {

	const funcName = "setAltSignAddr"

	// Get values which have been added to context by middleware.
	dcrdClient := c.MustGet(dcrdKey).(Node)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		log.Errorf("%s: could not get dcrd client: %v", funcName, dcrdErr.(error))
		s.sendError(errInternalError, c)
		return
	}
	reqBytes := c.MustGet(requestBytesKey).([]byte)

	if s.cfg.VspClosed {
		s.sendError(errVspClosed, c)
		return
	}

	var request setAltSignAddrRequest
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}

	altSignAddr, ticketHash := request.AltSignAddress, request.TicketHash

	currentData, err := s.db.AltSignAddrData(ticketHash)
	if err != nil {
		log.Errorf("%s: db.AltSignAddrData (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}
	if currentData != nil {
		msg := "alternate sign address data already exists"
		log.Warnf("%s: %s (ticketHash=%s)", funcName, msg, ticketHash)
		s.sendErrorWithMsg(msg, errBadRequest, c)
		return

	}

	// Fail fast if the pubkey doesn't decode properly.
	addr, err := stdaddr.DecodeAddressV0(altSignAddr, s.cfg.NetParams)
	if err != nil {
		log.Warnf("%s: Alt sign address cannot be decoded (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), errBadRequest, c)
		return
	}
	if _, ok := addr.(*stdaddr.AddressPubKeyHashEcdsaSecp256k1V0); !ok {
		log.Warnf("%s: Alt sign address is unexpected type (clientIP=%s, type=%T)", funcName, c.ClientIP(), addr)
		s.sendErrorWithMsg("wrong type for alternate signing address", errBadRequest, c)
		return
	}

	// Get ticket details.
	rawTicket, err := dcrdClient.GetRawTransaction(ticketHash)
	if err != nil {
		log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}

	// Ensure this ticket is eligible to vote at some point in the future.
	canVote, err := dcrdClient.CanTicketVote(rawTicket, s.cfg.NetParams)
	if err != nil {
		log.Errorf("%s: dcrd.CanTicketVote error (ticketHash=%s): %v", funcName, ticketHash, err)
		s.sendError(errInternalError, c)
		return
	}
	if !canVote {
		log.Warnf("%s: unvotable ticket (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), ticketHash)
		s.sendError(errTicketCannotVote, c)
		return
	}

	// Send success response to client.
	resp, respSig := s.sendJSONResponse(setAltSignAddrResponse{
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

	err = s.db.InsertAltSignAddr(ticketHash, data)
	if err != nil {
		log.Errorf("%s: db.InsertAltSignAddr error (ticketHash=%s): %v",
			funcName, ticketHash, err)
		return
	}

	log.Debugf("%s: New alt sign address set for ticket: (ticketHash=%s)", funcName, ticketHash)
}
