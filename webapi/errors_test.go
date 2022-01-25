// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"errors"
	"testing"
)

// TestApiErrorString tests the stringized output for the apiError type.
func TestApiErrorString(t *testing.T) {
	tests := []struct {
		in   apiError
		want string
	}{
		{errBadRequest, "bad request"},
		{errInternalError, "internal error"},
		{errVspClosed, "vsp is closed"},
		{errFeeAlreadyReceived, "fee tx already received for ticket"},
		{errInvalidFeeTx, "invalid fee tx"},
		{errFeeTooSmall, "fee too small"},
		{errUnknownTicket, "unknown ticket"},
		{errTicketCannotVote, "ticket not eligible to vote"},
		{errFeeExpired, "fee has expired"},
		{errInvalidVoteChoices, "invalid vote choices"},
		{errBadSignature, "bad request signature"},
		{errInvalidPrivKey, "invalid private key"},
		{errFeeNotReceived, "no fee tx received for ticket"},
		{errInvalidTicket, "not a valid ticket tx"},
		{errCannotBroadcastTicket, "ticket transaction could not be broadcast"},
		{errCannotBroadcastFee, "fee transaction could not be broadcast"},
		{errCannotBroadcastFeeUnknownOutputs, "fee transaction could not be broadcast due to unknown outputs"},
		{errInvalidTimestamp, "old or reused timestamp"},
	}

	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("%d: got: %s want: %s", i, result, test.want)
			continue
		}
	}
}

// TestApiErrorIsAs ensures apiError can be identified via errors.Is and unwrapped via errors.As.
func TestApiErrorIsAs(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		target    error
		wantMatch bool
		wantAs    apiError
	}{{
		name:      "errBadRequest == errBadRequest",
		err:       errBadRequest,
		target:    errBadRequest,
		wantMatch: true,
		wantAs:    errBadRequest,
	}, {
		name:      "errBadRequest != errInternalError",
		err:       errBadRequest,
		target:    errInternalError,
		wantMatch: false,
		wantAs:    errBadRequest,
	}}
	for _, test := range tests {
		// Ensure the error matches or not depending on the expected result.
		result := errors.Is(test.err, test.target)
		if result != test.wantMatch {
			t.Errorf("%s: incorrect error identification -- got %v, want %v",
				test.name, result, test.wantMatch)
			continue
		}

		// Ensure the underlying apiError can be unwrapped and is the
		// expected type.
		var err apiError
		if !errors.As(test.err, &err) {
			t.Errorf("%s: unable to unwrap error", test.name)
			continue
		}
		if err != test.wantAs {
			t.Errorf("%s: unexpected unwrapped error -- got %v, want %v",
				test.name, err, test.wantAs)
			continue
		}
	}
}
