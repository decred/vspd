package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/decred/slog"
	"github.com/decred/vspd/types"
)

// TestErrorDetails ensures errors returned by client.do contain adequate
// information for debugging (HTTP status and response body).
func TestErrorDetails(t *testing.T) {

	tests := map[string]struct {
		httpStatus        int
		responseBodyBytes []byte
		expectedErr       string
		vspdError         bool
	}{
		"500, vspd error": {
			httpStatus:        500,
			responseBodyBytes: []byte(`{"code": 1, "message": "bad request"}`),
			expectedErr:       `bad request`,
			vspdError:         true,
		},
		"500, no body": {
			httpStatus:        500,
			responseBodyBytes: nil,
			expectedErr:       `http status 500 (Internal Server Error) with no body`,
			vspdError:         false,
		},
		"500, non vspd error": {
			httpStatus:        500,
			responseBodyBytes: []byte(`an error occurred`),
			expectedErr:       `http status 500 (Internal Server Error) with body "an error occurred"`,
			vspdError:         false,
		},
		"500, non vspd error (json)": {
			httpStatus:        500,
			responseBodyBytes: []byte(`{"some": "json"}`),
			expectedErr:       `http status 500 (Internal Server Error) with body "{\"some\": \"json\"}"`,
			vspdError:         false,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {

			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				res.WriteHeader(testData.httpStatus)
				_, err := res.Write(testData.responseBodyBytes)
				if err != nil {
					t.Fatalf("writing response body failed: %v", err)
				}
			}))

			client := Client{
				URL:    testServer.URL,
				PubKey: []byte("fake pubkey"),
				Log:    slog.Disabled,
			}

			var resp interface{}
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
			}

		})
	}
}
