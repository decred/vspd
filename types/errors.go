// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import "net/http"

// ErrorCode is an integer which represents a kind of error which may be
// encountered by vspd.
type ErrorCode int64

const (
	ErrBadRequest ErrorCode = iota
	ErrInternalError
	ErrVspClosed
	ErrFeeAlreadyReceived
	ErrInvalidFeeTx
	ErrFeeTooSmall
	ErrUnknownTicket
	ErrTicketCannotVote
	ErrFeeExpired
	ErrInvalidVoteChoices
	ErrBadSignature
	ErrInvalidPrivKey
	ErrFeeNotReceived
	ErrInvalidTicket
	ErrCannotBroadcastTicket
	ErrCannotBroadcastFee
	ErrCannotBroadcastFeeUnknownOutputs
	ErrInvalidTimestamp
)

// HTTPStatus returns a corresponding HTTP status code for a given error code.
func (e ErrorCode) HTTPStatus() int {
	switch e {
	case ErrBadRequest:
		return http.StatusBadRequest
	case ErrInternalError:
		return http.StatusInternalServerError
	case ErrVspClosed:
		return http.StatusBadRequest
	case ErrFeeAlreadyReceived:
		return http.StatusBadRequest
	case ErrInvalidFeeTx:
		return http.StatusBadRequest
	case ErrFeeTooSmall:
		return http.StatusBadRequest
	case ErrUnknownTicket:
		return http.StatusBadRequest
	case ErrTicketCannotVote:
		return http.StatusBadRequest
	case ErrFeeExpired:
		return http.StatusBadRequest
	case ErrInvalidVoteChoices:
		return http.StatusBadRequest
	case ErrBadSignature:
		return http.StatusBadRequest
	case ErrInvalidPrivKey:
		return http.StatusBadRequest
	case ErrFeeNotReceived:
		return http.StatusBadRequest
	case ErrInvalidTicket:
		return http.StatusBadRequest
	case ErrCannotBroadcastTicket:
		return http.StatusInternalServerError
	case ErrCannotBroadcastFee:
		return http.StatusInternalServerError
	case ErrCannotBroadcastFeeUnknownOutputs:
		return http.StatusPreconditionRequired
	case ErrInvalidTimestamp:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// DefaultMessage returns a descriptive error string for a given error code.
func (e ErrorCode) DefaultMessage() string {
	switch e {
	case ErrBadRequest:
		return "bad request"
	case ErrInternalError:
		return "internal error"
	case ErrVspClosed:
		return "vsp is closed"
	case ErrFeeAlreadyReceived:
		return "fee tx already received for ticket"
	case ErrInvalidFeeTx:
		return "invalid fee tx"
	case ErrFeeTooSmall:
		return "fee too small"
	case ErrUnknownTicket:
		return "unknown ticket"
	case ErrTicketCannotVote:
		return "ticket not eligible to vote"
	case ErrFeeExpired:
		return "fee has expired"
	case ErrInvalidVoteChoices:
		return "invalid vote choices"
	case ErrBadSignature:
		return "bad request signature"
	case ErrInvalidPrivKey:
		return "invalid private key"
	case ErrFeeNotReceived:
		return "no fee tx received for ticket"
	case ErrInvalidTicket:
		return "not a valid ticket tx"
	case ErrCannotBroadcastTicket:
		return "ticket transaction could not be broadcast"
	case ErrCannotBroadcastFee:
		return "fee transaction could not be broadcast"
	case ErrCannotBroadcastFeeUnknownOutputs:
		return "fee transaction could not be broadcast due to unknown outputs"
	case ErrInvalidTimestamp:
		return "old or reused timestamp"
	default:
		return "unknown error"
	}
}
