// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"net/http"
	"testing"
)

// TestErrorDefaultMessages ensures each ErrorKind can be mapped to a default
// descriptive error message.
func TestErrorDefaultMessages(t *testing.T) {
	tests := []struct {
		in      ErrorCode
		wantMsg string
	}{
		{ErrBadRequest, "bad request"},
		{ErrInternalError, "internal error"},
		{ErrVspClosed, "vsp is closed"},
		{ErrFeeAlreadyReceived, "fee tx already received for ticket"},
		{ErrInvalidFeeTx, "invalid fee tx"},
		{ErrFeeTooSmall, "fee too small"},
		{ErrUnknownTicket, "unknown ticket"},
		{ErrTicketCannotVote, "ticket not eligible to vote"},
		{ErrFeeExpired, "fee has expired"},
		{ErrInvalidVoteChoices, "invalid vote choices"},
		{ErrBadSignature, "bad request signature"},
		{ErrInvalidPrivKey, "invalid private key"},
		{ErrFeeNotReceived, "no fee tx received for ticket"},
		{ErrInvalidTicket, "not a valid ticket tx"},
		{ErrCannotBroadcastTicket, "ticket transaction could not be broadcast"},
		{ErrCannotBroadcastFee, "fee transaction could not be broadcast"},
		{ErrCannotBroadcastFeeUnknownOutputs, "fee transaction could not be broadcast due to unknown outputs"},
		{ErrInvalidTimestamp, "old or reused timestamp"},
		{ErrorCode(9999), "unknown error"},
	}

	for _, test := range tests {
		actualMsg := test.in.DefaultMessage()
		if actualMsg != test.wantMsg {
			t.Errorf("wrong default message for ErrorKind(%d). expected: %q actual: %q ",
				test.in, test.wantMsg, actualMsg)
			continue
		}
	}
}

// TestErrorHTTPStatus ensures each ErrorCode can be mapped to a corresponding HTTP status code.
func TestErrorHTTPStatus(t *testing.T) {
	tests := []struct {
		in         ErrorCode
		wantStatus int
	}{
		{ErrBadRequest, http.StatusBadRequest},
		{ErrInternalError, http.StatusInternalServerError},
		{ErrVspClosed, http.StatusBadRequest},
		{ErrFeeAlreadyReceived, http.StatusBadRequest},
		{ErrInvalidFeeTx, http.StatusBadRequest},
		{ErrFeeTooSmall, http.StatusBadRequest},
		{ErrUnknownTicket, http.StatusBadRequest},
		{ErrTicketCannotVote, http.StatusBadRequest},
		{ErrFeeExpired, http.StatusBadRequest},
		{ErrInvalidVoteChoices, http.StatusBadRequest},
		{ErrBadSignature, http.StatusBadRequest},
		{ErrInvalidPrivKey, http.StatusBadRequest},
		{ErrFeeNotReceived, http.StatusBadRequest},
		{ErrInvalidTicket, http.StatusBadRequest},
		{ErrCannotBroadcastTicket, http.StatusInternalServerError},
		{ErrCannotBroadcastFee, http.StatusInternalServerError},
		{ErrCannotBroadcastFeeUnknownOutputs, http.StatusPreconditionRequired},
		{ErrInvalidTimestamp, http.StatusBadRequest},
		{ErrorCode(9999), http.StatusInternalServerError},
	}

	for _, test := range tests {
		result := test.in.HTTPStatus()
		if result != test.wantStatus {
			t.Errorf("wrong HTTP status for ErrorKind(%d). expected: %d actual: %d ",
				test.in, test.wantStatus, result)
			continue
		}
	}
}
