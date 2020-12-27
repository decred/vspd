// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import "net/http"

type apiError int

const (
	errBadRequest apiError = iota
	errInternalError
	errVspClosed
	errFeeAlreadyReceived
	errInvalidFeeTx
	errFeeTooSmall
	errUnknownTicket
	errTicketCannotVote
	errFeeExpired
	errInvalidVoteChoices
	errBadSignature
	errInvalidPrivKey
	errFeeNotReceived
	errInvalidTicket
	errCannotBroadcastTicket
	errCannotBroadcastFee
	errCannotBroadcastFeeUnknownOutputs
	errInvalidTimestamp
)

// httpStatus maps application error codes to HTTP status codes.
func (e apiError) httpStatus() int {
	switch e {
	case errBadRequest:
		return http.StatusBadRequest
	case errInternalError:
		return http.StatusInternalServerError
	case errVspClosed:
		return http.StatusBadRequest
	case errFeeAlreadyReceived:
		return http.StatusBadRequest
	case errInvalidFeeTx:
		return http.StatusBadRequest
	case errFeeTooSmall:
		return http.StatusBadRequest
	case errUnknownTicket:
		return http.StatusBadRequest
	case errTicketCannotVote:
		return http.StatusBadRequest
	case errFeeExpired:
		return http.StatusBadRequest
	case errInvalidVoteChoices:
		return http.StatusBadRequest
	case errBadSignature:
		return http.StatusBadRequest
	case errInvalidPrivKey:
		return http.StatusBadRequest
	case errFeeNotReceived:
		return http.StatusBadRequest
	case errInvalidTicket:
		return http.StatusBadRequest
	case errCannotBroadcastTicket:
		return http.StatusInternalServerError
	case errCannotBroadcastFee:
		return http.StatusInternalServerError
	case errCannotBroadcastFeeUnknownOutputs:
		return http.StatusPreconditionRequired
	case errInvalidTimestamp:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// defaultMessage returns a descriptive error string for a given error code.
func (e apiError) defaultMessage() string {
	switch e {
	case errBadRequest:
		return "bad request"
	case errInternalError:
		return "internal error"
	case errVspClosed:
		return "vsp is closed"
	case errFeeAlreadyReceived:
		return "fee tx already received for ticket"
	case errInvalidFeeTx:
		return "invalid fee tx"
	case errFeeTooSmall:
		return "fee too small"
	case errUnknownTicket:
		return "unknown ticket"
	case errTicketCannotVote:
		return "ticket not eligible to vote"
	case errFeeExpired:
		return "fee has expired"
	case errInvalidVoteChoices:
		return "invalid vote choices"
	case errBadSignature:
		return "bad request signature"
	case errInvalidPrivKey:
		return "invalid private key"
	case errFeeNotReceived:
		return "no fee tx received for ticket"
	case errInvalidTicket:
		return "not a valid ticket tx"
	case errCannotBroadcastTicket:
		return "ticket transaction could not be broadcast"
	case errCannotBroadcastFee:
		return "fee transaction could not be broadcast"
	case errCannotBroadcastFeeUnknownOutputs:
		return "fee transaction could not be broadcast due to unknown outputs"
	case errInvalidTimestamp:
		return "old or reused timestamp"
	default:
		return "unknown error"
	}
}
