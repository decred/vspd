// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v4"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/types"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gorilla/sessions"
	"github.com/jrick/wsrpc/v2"
)

// TicketSearchMessageFmt is the format for the message to be signed
// in order to search for a ticket using the vspd frontend.
const TicketSearchMessageFmt = "I want to check vspd ticket status for ticket %s at vsp with pubkey %s on window %d."

// withSession middleware adds a gorilla session to the request context for
// downstream handlers to make use of. Sessions are used by admin pages to
// maintain authentication status.
func (s *Server) withSession(store *sessions.CookieStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := store.Get(c.Request, "vspd-session")
		if err != nil {
			// "value is not valid" occurs if the cookie secret changes. This is
			// common during development (eg. when using the test harness) but
			// it should not occur in production.
			if strings.Contains(err.Error(), "securecookie: the value is not valid") {
				s.log.Warn("Cookie secret has changed. Generating new session.")

				// Persist the newly generated session.
				err = store.Save(c.Request, c.Writer, session)
				if err != nil {
					s.log.Errorf("Error saving session: %v", err)
					c.String(http.StatusInternalServerError, "Error saving session")
					c.Abort()
					return
				}
			} else {
				s.log.Errorf("Session error: %v", err)
				c.String(http.StatusInternalServerError, "Error getting session")
				c.Abort()
				return
			}
		}

		c.Set(sessionKey, session)
	}
}

// requireAdmin will only allow the request to proceed if the current session is
// authenticated as an admin, otherwise it will render the login template.
func (s *Server) requireAdmin(c *gin.Context) {
	session := c.MustGet(sessionKey).(*sessions.Session)
	admin := session.Values["admin"]

	if admin == nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"WebApiCache": s.cache.getData(),
			"WebApiCfg":   s.cfg,
		})
		c.Abort()
		return
	}
}

// withDcrdClient middleware adds a dcrd client to the request context for
// downstream handlers to make use of.
func (s *Server) withDcrdClient(dcrd rpc.DcrdConnect) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, hostname, err := dcrd.Client()
		// Don't handle the error here, add it to the context and let downstream
		// handlers decide what to do with it.
		c.Set(dcrdKey, client)
		c.Set(dcrdHostKey, hostname)
		c.Set(dcrdErrorKey, err)
	}
}

// withWalletClients middleware attempts to add voting wallet clients to the
// request context for downstream handlers to make use of. Downstream handlers
// must handle the case where no wallet clients are connected.
func (s *Server) withWalletClients(wallets rpc.WalletConnect) gin.HandlerFunc {
	return func(c *gin.Context) {
		clients, failedConnections := wallets.Clients()
		if len(clients) == 0 {
			s.log.Error("Could not connect to any wallets")
		} else if len(failedConnections) > 0 {
			s.log.Errorf("Failed to connect to %d wallet(s), proceeding with only %d",
				len(failedConnections), len(clients))
		}
		c.Set(walletsKey, clients)
		c.Set(failedWalletsKey, failedConnections)
	}
}

// drainAndReplaceBody will read and return the body of the provided request. It
// replaces the request reader with an identical one so it can be used again.
func drainAndReplaceBody(req *http.Request) ([]byte, error) {
	reqBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewBuffer(reqBytes))
	return reqBytes, nil
}

func (s *Server) vspMustBeOpen(c *gin.Context) {
	if s.cfg.VspClosed {
		s.sendError(types.ErrVspClosed, c)
		return
	}
}

// broadcastTicket will ensure that the local dcrd instance is aware of the
// provided ticket.
// Ticket hash, ticket hex, and parent hex are parsed from the request body and
// validated. They are broadcast to the network using SendRawTransaction if dcrd
// is not aware of them.
func (s *Server) broadcastTicket(c *gin.Context) {
	const funcName = "broadcastTicket"

	// Read request bytes.
	reqBytes, err := drainAndReplaceBody(c.Request)
	if err != nil {
		s.log.Warnf("%s: Error reading request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	// Parse request to ensure ticket hash/hex and parent hex are included.
	var request struct {
		TicketHex  string `json:"tickethex" binding:"required"`
		TicketHash string `json:"tickethash" binding:"required"`
		ParentHex  string `json:"parenthex" binding:"required"`
	}
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		s.log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	// Ensure the provided ticket hex is a valid ticket.
	msgTx, err := decodeTransaction(request.TicketHex)
	if err != nil {
		s.log.Errorf("%s: Failed to decode ticket hex (ticketHash=%s): %v",
			funcName, request.TicketHash, err)
		s.sendErrorWithMsg("cannot decode ticket hex", types.ErrBadRequest, c)
		return
	}

	err = isValidTicket(msgTx)
	if err != nil {
		s.log.Warnf("%s: Invalid ticket (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), request.TicketHash, err)
		s.sendError(types.ErrInvalidTicket, c)
		return
	}

	// Ensure hex matches hash.
	if msgTx.TxHash().String() != request.TicketHash {
		s.log.Warnf("%s: Ticket hex/hash mismatch (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), request.TicketHash)
		s.sendErrorWithMsg("ticket hex does not match hash", types.ErrBadRequest, c)
		return
	}

	// Ensure the provided parent hex is a valid tx.
	parentTx, err := decodeTransaction(request.ParentHex)
	if err != nil {
		s.log.Errorf("%s: Failed to decode parent hex (ticketHash=%s): %v", funcName, request.TicketHash, err)
		s.sendErrorWithMsg("cannot decode parent hex", types.ErrBadRequest, c)
		return
	}
	parentHash := parentTx.TxHash()

	// Check if local dcrd already knows the parent tx.
	dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		s.log.Errorf("%s: Could not get dcrd client: %v", funcName, dcrdErr.(error))
		s.sendError(types.ErrInternalError, c)
		return
	}

	_, err = dcrdClient.GetRawTransaction(parentHash.String())
	var e *wsrpc.Error
	if err == nil {
		// No error means dcrd already knows the parent tx, nothing to do.
	} else if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
		// ErrNoTxInfo means local dcrd is not aware of the parent. We have
		// the hex, so we can broadcast it here.

		// Before broadcasting, check that the provided parent hex is
		// actually the parent of the ticket.
		var found bool
		for _, txIn := range msgTx.TxIn {
			if !txIn.PreviousOutPoint.Hash.IsEqual(&parentHash) {
				continue
			}
			found = true
			break
		}

		if !found {
			s.log.Errorf("%s: Invalid ticket parent (ticketHash=%s)", funcName, request.TicketHash)
			s.sendErrorWithMsg("invalid ticket parent", types.ErrBadRequest, c)
			return
		}

		s.log.Debugf("%s: Broadcasting parent tx %s (ticketHash=%s)", funcName, parentHash, request.TicketHash)
		err = dcrdClient.SendRawTransaction(request.ParentHex)
		if err != nil {
			s.log.Errorf("%s: dcrd.SendRawTransaction for parent tx failed (ticketHash=%s): %v",
				funcName, request.TicketHash, err)
			s.sendError(types.ErrCannotBroadcastTicket, c)
			return
		}

	} else {
		s.log.Errorf("%s: dcrd.GetRawTransaction for ticket parent failed (ticketHash=%s): %v",
			funcName, request.TicketHash, err)
		s.sendError(types.ErrInternalError, c)
		return
	}

	// Check if local dcrd already knows the ticket.
	_, err = dcrdClient.GetRawTransaction(request.TicketHash)
	if err == nil {
		// No error means dcrd already knows the ticket, we are done here.
		return
	}

	// ErrNoTxInfo means local dcrd is not aware of the ticket. We have the
	// hex, so we can broadcast it here.
	if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
		s.log.Debugf("%s: Broadcasting ticket (ticketHash=%s)", funcName, request.TicketHash)
		err = dcrdClient.SendRawTransaction(request.TicketHex)
		if err != nil {
			s.log.Errorf("%s: dcrd.SendRawTransaction for ticket failed (ticketHash=%s): %v",
				funcName, request.TicketHash, err)
			s.sendError(types.ErrCannotBroadcastTicket, c)
			return
		}
	} else {
		s.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
			funcName, request.TicketHash, err)
		s.sendError(types.ErrInternalError, c)
		return
	}
}

// vspAuth middleware reads the request body and extracts the ticket hash. The
// commitment address for the ticket is retrieved from the database if it is
// known, or it is retrieved from the chain if not.
// The middleware errors out if the VSP-Client-Signature header of the request
// does not contain the request body signed with the commitment address.
// Ticket information is added to the request context for downstream handlers to
// use.
func (s *Server) vspAuth(c *gin.Context) {
	const funcName = "vspAuth"

	// Read request bytes.
	reqBytes, err := drainAndReplaceBody(c.Request)
	if err != nil {
		s.log.Warnf("%s: Error reading request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	// Add request bytes to request context for downstream handlers to reuse.
	// Necessary because the request body reader can only be used once.
	c.Set(requestBytesKey, reqBytes)

	// Parse request and ensure there is a ticket hash included.
	var request struct {
		TicketHash string `json:"tickethash" binding:"required"`
	}
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		s.log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}
	hash := request.TicketHash

	// Before hitting the db or any RPC, ensure this is a valid ticket hash.
	err = validateTicketHash(hash)
	if err != nil {
		s.log.Errorf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		s.sendErrorWithMsg("invalid ticket hash", types.ErrBadRequest, c)
		return
	}

	// Check if this ticket already appears in the database.
	ticket, ticketFound, err := s.db.GetTicketByHash(hash)
	if err != nil {
		s.log.Errorf("%s: db.GetTicketByHash error (ticketHash=%s): %v", funcName, hash, err)
		s.sendError(types.ErrInternalError, c)
		return
	}

	var commitmentAddress string
	if ticketFound {
		// The commitment address is already known if the ticket already exists
		// in the database.
		commitmentAddress = ticket.CommitmentAddress
	} else {
		// Otherwise the commitment address must be retrieved from the chain
		// using dcrd.
		dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
		dcrdErr := c.MustGet(dcrdErrorKey)
		if dcrdErr != nil {
			s.log.Errorf("%s: Could not get dcrd client (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, dcrdErr.(error))
			s.sendError(types.ErrInternalError, c)
			return
		}

		rawTx, err := dcrdClient.GetRawTransaction(hash)
		if err != nil {
			s.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			s.sendError(types.ErrInternalError, c)
			return
		}

		msgTx, err := decodeTransaction(rawTx.Hex)
		if err != nil {
			s.log.Errorf("%s: Failed to decode ticket hex (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			s.sendError(types.ErrInternalError, c)
			return
		}

		err = isValidTicket(msgTx)
		if err != nil {
			s.log.Errorf("%s: Invalid ticket (clientIP=%s, ticketHash=%s)",
				funcName, c.ClientIP(), hash)
			s.sendError(types.ErrInvalidTicket, c)
			return
		}

		addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, s.cfg.NetParams)
		if err != nil {
			s.log.Errorf("%s: AddrFromSStxPkScrCommitment error (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			s.sendError(types.ErrInternalError, c)
			return
		}

		commitmentAddress = addr.String()
	}

	// Ensure a signature is provided.
	signature := c.GetHeader("VSP-Client-Signature")
	if signature == "" {
		s.log.Warnf("%s: No VSP-Client-Signature header (clientIP=%s)", funcName, c.ClientIP())
		s.sendErrorWithMsg("no VSP-Client-Signature header", types.ErrBadRequest, c)
		return
	}

	// Validate request signature to ensure ticket ownership.
	err = validateSignature(hash, commitmentAddress, signature, string(reqBytes), s.db, s.cfg.NetParams)
	if err != nil {
		s.log.Errorf("%s: Couldn't validate signature (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), hash, err)
		s.sendError(types.ErrBadSignature, c)
		return
	}

	// Add ticket information to context so downstream handlers don't need
	// to access the db for it.
	c.Set(ticketKey, ticket)
	c.Set(knownTicketKey, ticketFound)
	c.Set(commitmentAddressKey, commitmentAddress)
}

// ticketSearchAuth middleware reads the request form body and extracts the
// ticket hash and signature from the base64 string provided. The commitment
// address for the ticket is retrieved from the database if it is known, or it
// is retrieved from the chain if not. The middleware errors out if required
// information is not provided or the signature does not contain a message
// signed with the commitment address. Ticket information is added to the
// request context for downstream handlers to use.
func (s *Server) ticketSearchAuth(c *gin.Context) {
	funcName := "ticketSearchAuth"

	encodedString := c.PostForm("encoded")

	// Get information added to context.
	dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		s.log.Errorf("%s: Could not get dcrd client: %v", funcName, dcrdErr.(error))
		c.Set(errorKey, errInternalError)
		return
	}

	currentBlockHeader, err := dcrdClient.GetBestBlockHeader()
	if err != nil {
		s.log.Errorf("%s: Error getting best block header : %v", funcName, err)
		c.Set(errorKey, errInternalError)
		// Average blocks per day for the current network.
		blocksPerDay := (24 * time.Hour) / s.cfg.NetParams.TargetTimePerBlock
		blockWindow := int(currentBlockHeader.Height) / int(blocksPerDay)

		decodedByte, err := base64.StdEncoding.DecodeString(encodedString)
		if err != nil {
			s.log.Errorf("%s: Decoding form data error : %v", funcName, err)
			c.Set(errorKey, errBadRequest)
			return
		}

		data := strings.Split(string(decodedByte), ":")
		if len(data) != 2 {
			c.Set(errorKey, errBadRequest)
			return
		}

		ticketHash := data[0]
		signature := data[1]
		vspPublicKey := s.cache.data.PubKey
		messageSigned := fmt.Sprintf(TicketSearchMessageFmt, ticketHash, vspPublicKey, blockWindow)

		// Before hitting the db or any RPC, ensure this is a valid ticket hash.
		err = validateTicketHash(ticketHash)
		if err != nil {
			s.log.Errorf("%s: Invalid ticket (clientIP=%s): %v", funcName, c.ClientIP(), err)
			c.Set(errorKey, errInvalidTicket)
			return
		}

		// Check if this ticket already appears in the database.
		ticket, ticketFound, err := s.db.GetTicketByHash(ticketHash)
		if err != nil {
			s.log.Errorf("%s: db.GetTicketByHash error (ticketHash=%s): %v", funcName, ticketHash, err)
			c.Set(errorKey, errInternalError)
			return
		}

		if !ticketFound {
			s.log.Warnf("%s: Unknown ticket (clientIP=%s)", funcName, c.ClientIP())
			c.Set(errorKey, errUnknownTicket)
			return
		}

		// If the ticket was found in the database, we already know its
		// commitment address. Otherwise we need to get it from the chain.
		var commitmentAddress string
		if ticketFound {
			commitmentAddress = ticket.CommitmentAddress
		} else {
			commitmentAddress, err = getCommitmentAddress(ticketHash, dcrdClient, s.cfg.NetParams)
			if err != nil {
				s.log.Errorf("%s: Failed to get commitment address (clientIP=%s, ticketHash=%s): %v",
					funcName, c.ClientIP(), ticketHash, err)

				var apiErr *apiError
				if errors.Is(err, apiErr) {
					c.Set(errorKey, errInvalidTicket)
				} else {
					c.Set(errorKey, errInternalError)
				}

				return
			}
		}

		// Validate request signature to ensure ticket ownership.
		err = validateSignature(ticketHash, commitmentAddress, signature, messageSigned, s.db, s.cfg.NetParams)
		if err != nil {
			s.log.Errorf("%s: Couldn't validate signature (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), ticketHash, err)
			c.Set(errorKey, errBadSignature)
			return
		}

		// Add ticket information to context so downstream handlers don't need
		// to access the db for it.
		c.Set(ticketKey, ticket)
		c.Set(knownTicketKey, ticketFound)
		c.Set(errorKey, nil)
	}
}
