// Copyright (c) 2020-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/decred/dcrd/blockchain/stake/v5"
	"github.com/decred/vspd/rpc"
	"github.com/decred/vspd/types/v3"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gorilla/sessions"
	"github.com/jrick/wsrpc/v2"
	"golang.org/x/time/rate"
)

// This is a hard-coded string from the securecookie library.
const invalidCookieErr = "securecookie: the value is not valid"

// rateLimit middleware limits how many requests each client IP can submit per
// second. If the limit is exceeded the limitExceeded handler will be executed
// and the context will be aborted.
func rateLimit(limit rate.Limit, limitExceeded gin.HandlerFunc) gin.HandlerFunc {
	var limitersMtx sync.Mutex
	limiters := make(map[string]*rate.Limiter)

	// Clear limiters every hour so they arent accumulating infinitely.
	go func() {
		for {
			<-time.After(time.Hour)
			limitersMtx.Lock()
			limiters = make(map[string]*rate.Limiter)
			limitersMtx.Unlock()
		}
	}()

	return func(c *gin.Context) {
		limitersMtx.Lock()
		defer limitersMtx.Unlock()

		// Create a limiter for this IP if one does not exist.
		if _, ok := limiters[c.ClientIP()]; !ok {
			limiters[c.ClientIP()] = rate.NewLimiter(limit, 1)
		}

		// Check if this IP exceeds limit.
		if !limiters[c.ClientIP()].Allow() {
			limitExceeded(c)
			c.Abort()
		}
	}
}

// withSession middleware adds a gorilla session to the request context for
// downstream handlers to make use of. Sessions are used by admin pages to
// maintain authentication status.
func (w *WebAPI) withSession(store *sessions.CookieStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, err := store.Get(c.Request, "vspd-session")
		if err != nil {
			// "value is not valid" occurs if the cookie secret changes. This is
			// common during development (eg. when using the test harness) but
			// it should not occur in production.
			if strings.Contains(err.Error(), invalidCookieErr) {
				w.log.Warn("Cookie secret has changed. Generating new session.")

				// Persist the newly generated session.
				err = store.Save(c.Request, c.Writer, session)
				if err != nil {
					w.log.Errorf("Error saving session: %v", err)
					c.String(http.StatusInternalServerError, "Error saving session")
					c.Abort()
					return
				}
			} else {
				w.log.Errorf("Session error: %v", err)
				c.String(http.StatusInternalServerError, "Error getting session")
				c.Abort()
				return
			}
		}

		c.Set(sessionKey, session)
	}
}

// requireWebCache will only allow the request to proceed if the web API cache
// has been initialized with data, otherwise it will return a 500 Internal
// Server Error.
func (w *WebAPI) requireWebCache(c *gin.Context) {
	if !w.cache.initialized() {
		// Try to initialize it now.
		err := w.cache.update()
		if err != nil {
			w.log.Errorf("Failed to initialize cache: %v", err)
			c.String(http.StatusInternalServerError, "Cache is not initialized")
			c.Abort()
			return
		}
	}

	c.Set(cacheKey, w.cache.getData())
}

// requireAdmin will only allow the request to proceed if the current session is
// authenticated as an admin, otherwise it will render the login template.
func (w *WebAPI) requireAdmin(c *gin.Context) {
	cacheData := c.MustGet(cacheKey).(cacheData)
	session := c.MustGet(sessionKey).(*sessions.Session)
	admin := session.Values["admin"]

	if admin == nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{
			"WebApiCache": cacheData,
			"WebApiCfg":   w.cfg,
		})
		c.Abort()
		return
	}
}

// withDcrdClient middleware adds a dcrd client to the request context for
// downstream handlers to make use of.
func (w *WebAPI) withDcrdClient(dcrd rpc.DcrdConnect) gin.HandlerFunc {
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
func (w *WebAPI) withWalletClients(wallets rpc.WalletConnect) gin.HandlerFunc {
	return func(c *gin.Context) {
		clients, failedConnections := wallets.Clients()
		if len(clients) == 0 {
			w.log.Error("Could not connect to any wallets")
		} else if len(failedConnections) > 0 {
			w.log.Errorf("Failed to connect to %d wallet(s), proceeding with only %d",
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

func (w *WebAPI) vspMustBeOpen(c *gin.Context) {
	if w.cfg.VspClosed {
		w.sendError(types.ErrVspClosed, c)
		return
	}
}

// broadcastTicket will ensure that the local dcrd instance is aware of the
// provided ticket.
// Ticket hash, ticket hex, and parent hex are parsed from the request body and
// validated. They are broadcast to the network using SendRawTransaction if dcrd
// is not aware of them.
func (w *WebAPI) broadcastTicket(c *gin.Context) {
	const funcName = "broadcastTicket"

	// Read request bytes.
	reqBytes, err := drainAndReplaceBody(c.Request)
	if err != nil {
		w.log.Warnf("%s: Error reading request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	// Parse request to ensure ticket hash/hex and parent hex are included.
	var request struct {
		TicketHex  string `json:"tickethex" binding:"required"`
		TicketHash string `json:"tickethash" binding:"required"`
		ParentHex  string `json:"parenthex" binding:"required"`
	}
	if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
		w.log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}

	// Ensure the provided ticket hex is a valid ticket.
	msgTx, err := decodeTransaction(request.TicketHex)
	if err != nil {
		w.log.Errorf("%s: Failed to decode ticket hex (ticketHash=%s): %v",
			funcName, request.TicketHash, err)
		w.sendErrorWithMsg("cannot decode ticket hex", types.ErrBadRequest, c)
		return
	}

	err = isValidTicket(msgTx)
	if err != nil {
		w.log.Warnf("%s: Invalid ticket (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), request.TicketHash, err)
		w.sendError(types.ErrInvalidTicket, c)
		return
	}

	// Ensure hex matches hash.
	if msgTx.TxHash().String() != request.TicketHash {
		w.log.Warnf("%s: Ticket hex/hash mismatch (clientIP=%s, ticketHash=%s)",
			funcName, c.ClientIP(), request.TicketHash)
		w.sendErrorWithMsg("ticket hex does not match hash", types.ErrBadRequest, c)
		return
	}

	// Ensure the provided parent hex is a valid tx.
	parentTx, err := decodeTransaction(request.ParentHex)
	if err != nil {
		w.log.Errorf("%s: Failed to decode parent hex (ticketHash=%s): %v", funcName, request.TicketHash, err)
		w.sendErrorWithMsg("cannot decode parent hex", types.ErrBadRequest, c)
		return
	}
	parentHash := parentTx.TxHash()

	// Check if local dcrd already knows the parent tx.
	dcrdClient := c.MustGet(dcrdKey).(*rpc.DcrdRPC)
	dcrdErr := c.MustGet(dcrdErrorKey)
	if dcrdErr != nil {
		w.log.Errorf("%s: %v", funcName, dcrdErr.(error))
		w.sendError(types.ErrInternalError, c)
		return
	}

	_, err = dcrdClient.GetRawTransaction(parentHash.String())
	if err != nil {
		var e *wsrpc.Error
		if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
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
				w.log.Errorf("%s: Invalid ticket parent (ticketHash=%s)", funcName, request.TicketHash)
				w.sendErrorWithMsg("invalid ticket parent", types.ErrBadRequest, c)
				return
			}

			w.log.Debugf("%s: Broadcasting parent tx %s (ticketHash=%s)", funcName, parentHash, request.TicketHash)
			err = dcrdClient.SendRawTransaction(request.ParentHex)
			if err != nil {
				// Unknown output errors have special handling because they
				// could be resolved by waiting for network propagation. Any
				// other errors are returned to client immediately.
				if !strings.Contains(err.Error(), rpc.ErrUnknownOutputs) {
					w.log.Errorf("%s: dcrd.SendRawTransaction for parent tx failed (ticketHash=%s): %v",
						funcName, request.TicketHash, err)
					w.sendError(types.ErrCannotBroadcastTicket, c)
					return
				}

				w.log.Debugf("%s: Parent tx references an unknown output, waiting for it in mempool (ticketHash=%s)",
					funcName, request.TicketHash)

				txBroadcast := func() bool {
					// Wait for 1 second and try again, max 7 attempts.
					for range 7 {
						time.Sleep(1 * time.Second)
						err := dcrdClient.SendRawTransaction(request.ParentHex)
						if err == nil {
							return true
						}
					}
					return false
				}()

				if !txBroadcast {
					w.log.Errorf("%s: Failed to broadcast parent tx, waiting didn't help (ticketHash=%s)",
						funcName, request.TicketHash)
					w.sendError(types.ErrCannotBroadcastTicket, c)
					return
				}
			}

		} else {
			w.log.Errorf("%s: dcrd.GetRawTransaction for ticket parent failed (ticketHash=%s): %v",
				funcName, request.TicketHash, err)
			w.sendError(types.ErrInternalError, c)
			return
		}
	}

	// Check if local dcrd already knows the ticket.
	_, err = dcrdClient.GetRawTransaction(request.TicketHash)
	if err == nil {
		// No error means dcrd already knows the ticket, we are done here.
		return
	}

	// ErrNoTxInfo means local dcrd is not aware of the ticket. We have the
	// hex, so we can broadcast it here.
	var e *wsrpc.Error
	if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
		w.log.Debugf("%s: Broadcasting ticket (ticketHash=%s)", funcName, request.TicketHash)
		err = dcrdClient.SendRawTransaction(request.TicketHex)
		if err != nil {
			w.log.Errorf("%s: dcrd.SendRawTransaction for ticket failed (ticketHash=%s): %v",
				funcName, request.TicketHash, err)
			w.sendError(types.ErrCannotBroadcastTicket, c)
			return
		}
	} else {
		w.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
			funcName, request.TicketHash, err)
		w.sendError(types.ErrInternalError, c)
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
func (w *WebAPI) vspAuth(c *gin.Context) {
	const funcName = "vspAuth"

	// Read request bytes.
	reqBytes, err := drainAndReplaceBody(c.Request)
	if err != nil {
		w.log.Warnf("%s: Error reading request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
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
		w.log.Warnf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg(err.Error(), types.ErrBadRequest, c)
		return
	}
	hash := request.TicketHash

	// Before hitting the db or any RPC, ensure this is a valid ticket hash.
	err = validateTicketHash(hash)
	if err != nil {
		w.log.Errorf("%s: Bad request (clientIP=%s): %v", funcName, c.ClientIP(), err)
		w.sendErrorWithMsg("invalid ticket hash", types.ErrBadRequest, c)
		return
	}

	// Check if this ticket already appears in the database.
	ticket, ticketFound, err := w.db.GetTicketByHash(hash)
	if err != nil {
		w.log.Errorf("%s: db.GetTicketByHash error (ticketHash=%s): %v", funcName, hash, err)
		w.sendError(types.ErrInternalError, c)
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
			w.log.Errorf("%s: Could not get dcrd client (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, dcrdErr.(error))
			w.sendError(types.ErrInternalError, c)
			return
		}

		rawTx, err := dcrdClient.GetRawTransaction(hash)
		if err != nil {
			w.log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			w.sendError(types.ErrInternalError, c)
			return
		}

		msgTx, err := decodeTransaction(rawTx.Hex)
		if err != nil {
			w.log.Errorf("%s: Failed to decode ticket hex (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			w.sendError(types.ErrInternalError, c)
			return
		}

		err = isValidTicket(msgTx)
		if err != nil {
			w.log.Errorf("%s: Invalid ticket (clientIP=%s, ticketHash=%s)",
				funcName, c.ClientIP(), hash)
			w.sendError(types.ErrInvalidTicket, c)
			return
		}

		addr, err := stake.AddrFromSStxPkScrCommitment(msgTx.TxOut[1].PkScript, w.cfg.Network)
		if err != nil {
			w.log.Errorf("%s: AddrFromSStxPkScrCommitment error (clientIP=%s, ticketHash=%s): %v",
				funcName, c.ClientIP(), hash, err)
			w.sendError(types.ErrInternalError, c)
			return
		}

		commitmentAddress = addr.String()
	}

	// Ensure a signature is provided.
	signature := c.GetHeader("VSP-Client-Signature")
	if signature == "" {
		w.log.Warnf("%s: No VSP-Client-Signature header (clientIP=%s)", funcName, c.ClientIP())
		w.sendErrorWithMsg("no VSP-Client-Signature header", types.ErrBadRequest, c)
		return
	}

	// Validate request signature to ensure ticket ownership.
	err = validateSignature(hash, commitmentAddress, signature, string(reqBytes), w.db, w.cfg.Network)
	if err != nil {
		w.log.Errorf("%s: Couldn't validate signature (clientIP=%s, ticketHash=%s): %v",
			funcName, c.ClientIP(), hash, err)
		w.sendError(types.ErrBadSignature, c)
		return
	}

	// Add ticket information to context so downstream handlers don't need
	// to access the db for it.
	c.Set(ticketKey, ticket)
	c.Set(knownTicketKey, ticketFound)
	c.Set(commitmentAddressKey, commitmentAddress)
}
