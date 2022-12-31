// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

import (
	"errors"
	"testing"
)

// TestAPIErrorAs ensures APIError can be unwrapped via errors.As.
func TestAPIErrorAs(t *testing.T) {

	tests := map[string]struct {
		apiError        error
		expectedKind    ErrorCode
		expectedMessage string
	}{
		"BadRequest error": {
			apiError:        ErrorResponse{Message: "something went wrong", Code: int64(ErrBadRequest)},
			expectedKind:    ErrBadRequest,
			expectedMessage: "something went wrong",
		},
		"Unknown error": {
			apiError:        ErrorResponse{Message: "something went wrong again", Code: int64(999)},
			expectedKind:    999,
			expectedMessage: "something went wrong again",
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {

			// Ensure APIError can be unwrapped from error.
			var parsedError ErrorResponse
			if !errors.As(test.apiError, &parsedError) {
				t.Fatalf("unable to unwrap error")
			}

			if parsedError.Code != int64(test.expectedKind) {
				t.Fatalf("error was wrong kind. expected: %d actual %d",
					test.expectedKind, parsedError.Code)
			}

			if parsedError.Message != test.expectedMessage {
				t.Fatalf("error had wrong message. expected: %q actual %q",
					test.expectedMessage, parsedError.Message)
			}

		})
	}
}
