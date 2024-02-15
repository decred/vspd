// Copyright (c) 2022-2024 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/decred/slog"
	"github.com/decred/vspd/types/v2"
)

// TestErrorDetails ensures errors returned by client.do contain adequate
// information for debugging (HTTP status and response body).
func TestErrorDetails(t *testing.T) {

	tests := map[string]struct {
		respHTTPStatus int
		respBodyBytes  []byte
		expectedErr    string
		vspdError      bool
		vspdErrCode    types.ErrorCode
	}{
		"500, vspd error (generic bad request)": {
			respHTTPStatus: 500,
			respBodyBytes:  []byte(`{"code": 0, "message": "bad request"}`),
			expectedErr:    `bad request`,
			vspdError:      true,
			vspdErrCode:    types.ErrBadRequest,
		},
		"500, vspd error (generic internal error)": {
			respHTTPStatus: 500,
			respBodyBytes:  []byte(`{"code": 1, "message": "something terrible happened"}`),
			expectedErr:    `something terrible happened`,
			vspdError:      true,
			vspdErrCode:    types.ErrInternalError,
		},
		"428, vspd error (cannot broadcast fee)": {
			respHTTPStatus: 428,
			respBodyBytes:  []byte(`{"code": 16, "message": "fee transaction could not be broadcast due to unknown outputs"}`),
			expectedErr:    `fee transaction could not be broadcast due to unknown outputs`,
			vspdError:      true,
			vspdErrCode:    types.ErrCannotBroadcastFeeUnknownOutputs,
		},
		"500, no body": {
			respHTTPStatus: 500,
			respBodyBytes:  nil,
			expectedErr:    `http status 500 (Internal Server Error) with no body`,
			vspdError:      false,
		},
		"500, non vspd error": {
			respHTTPStatus: 500,
			respBodyBytes:  []byte(`an error occurred`),
			expectedErr:    `http status 500 (Internal Server Error) with body "an error occurred"`,
			vspdError:      false,
		},
		"500, non vspd error (json)": {
			respHTTPStatus: 500,
			respBodyBytes:  []byte(`{"some": "json"}`),
			expectedErr:    `http status 500 (Internal Server Error) with body "{\"some\": \"json\"}"`,
			vspdError:      false,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {

			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
				res.WriteHeader(testData.respHTTPStatus)
				_, err := res.Write(testData.respBodyBytes)
				if err != nil {
					t.Fatalf("writing response body failed: %v", err)
				}
			}))

			client := Client{
				URL:    testServer.URL,
				PubKey: []byte("fake pubkey"),
				Log:    slog.Disabled,
			}

			var resp any
			err := client.do(context.TODO(), http.MethodGet, "", nil, &resp, nil)

			testServer.Close()

			if err == nil {
				t.Fatalf("client.do did not return an error")
			}

			if err.Error() != testData.expectedErr {
				t.Fatalf("client.do returned incorrect error, expected %q, got %q",
					testData.expectedErr, err.Error())
			}

			if testData.vspdError {
				// Error should be unwrappable as a vspd error response.
				var e types.ErrorResponse
				if !errors.As(err, &e) {
					t.Fatal("unable to unwrap vspd error")
				}

				if e.Code != testData.vspdErrCode {
					t.Fatalf("incorrect vspd error code, expected %d, got %d",
						testData.vspdErrCode, e.Code)
				}
			}

		})
	}
}

// TestSignatureValidation ensures that responses with invalid signatures are
// flagged.
func TestSignatureValidation(t *testing.T) {

	// Generate some test data for the valid signature case.
	privKey := ed25519.NewKeyFromSeed([]byte("00000000000000000000000000000000"))
	pubKey, _ := privKey.Public().(ed25519.PublicKey)
	emptyJSON := []byte("{}")
	validSig := base64.StdEncoding.EncodeToString(ed25519.Sign(privKey, emptyJSON))

	tests := map[string]struct {
		responseSig  string
		expectErr    bool
		expectErrStr string
	}{
		"valid signature": {
			responseSig: validSig,
			expectErr:   false,
		},
		"invalid signature": {
			responseSig:  "1234",
			expectErr:    true,
			expectErrStr: "authenticate server response: invalid signature",
		},
		"no signature": {
			responseSig:  "",
			expectErr:    true,
			expectErrStr: "authenticate server response: no signature provided",
		},
		"failed to decode signature": {
			responseSig:  "0xp",
			expectErr:    true,
			expectErrStr: "authenticate server response: failed to decode signature: illegal base64 data at input byte 0",
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {

			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, _ *http.Request) {
				res.Header().Add("VSP-Server-Signature", testData.responseSig)
				res.WriteHeader(http.StatusOK)
				_, err := res.Write(emptyJSON)
				if err != nil {
					t.Fatalf("writing response body failed: %v", err)
				}
			}))

			client := Client{
				URL:    testServer.URL,
				PubKey: pubKey,
				Log:    slog.Disabled,
			}

			var resp any
			err := client.do(context.TODO(), http.MethodGet, "", nil, &resp, nil)

			testServer.Close()

			if testData.expectErr {
				if err.Error() != testData.expectErrStr {
					t.Fatalf("client.do returned incorrect error, expected %q, got %q",
						testData.expectErrStr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("client.do returned unexpected error: %v", err)
				}
			}

		})
	}
}
