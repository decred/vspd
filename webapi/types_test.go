// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package webapi

import (
	"errors"
	"testing"
)

// TestAPIErrorAs ensures APIError can be unwrapped via errors.As.
func TestAPIErrorAs(t *testing.T) {

	tests := []struct {
		testName        string
		apiError        error
		expectedKind    ErrorCode
		expectedMessage string
	}{{
		testName:        "BadRequest error",
		apiError:        APIError{Message: "something went wrong", Code: int64(ErrBadRequest)},
		expectedKind:    ErrBadRequest,
		expectedMessage: "something went wrong",
	},
		{
			testName:        "Unknown error",
			apiError:        APIError{Message: "something went wrong again", Code: int64(999)},
			expectedKind:    999,
			expectedMessage: "something went wrong again",
		}}

	for _, test := range tests {
		// Ensure APIError can be unwrapped from error.
		var parsedError APIError
		if !errors.As(test.apiError, &parsedError) {
			t.Errorf("%s: unable to unwrap error", test.testName)
			continue
		}

		if parsedError.Code != int64(test.expectedKind) {
			t.Errorf("%s: error was wrong kind. expected: %d actual %d",
				test.testName, test.expectedKind, parsedError.Code)
			continue
		}

		if parsedError.Message != test.expectedMessage {
			t.Errorf("%s: error had wrong message. expected: %q actual %q",
				test.testName, test.expectedMessage, parsedError.Message)
			continue
		}
	}
}
