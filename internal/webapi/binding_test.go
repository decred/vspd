package webapi

import (
	"testing"

	"github.com/decred/vspd/types/v3"
	"github.com/gin-gonic/gin/binding"
)

// TestGinJSONBinding does not test code in this package. It is a
// characterization test to determine exactly how Gin handles JSON binding tags.
func TestGinJSONBinding(t *testing.T) {
	tests := map[string]struct {
		req         []byte
		expectedErr string
	}{

		"Filled arrays bind without error": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    {"k": "v"},
				"tspendpolicy":   {"k": "v"},
				"treasurypolicy": {"k": "v"}
			}`),
			expectedErr: "",
		},

		"Array filled beyond max does not bind": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    {"k": "v"},
				"tspendpolicy":   {"k": "v"},
				"treasurypolicy": {"k1": "v","k2": "v","k3": "v","k4": "v"}
			}`),
			expectedErr: "Key: 'SetVoteChoicesRequest.TreasuryPolicy' Error:Field validation for 'TreasuryPolicy' failed on the 'max' tag",
		},

		"Empty arrays bind without error": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    {},
				"tspendpolicy":   {},
				"treasurypolicy": {}
			}`),
			expectedErr: "",
		},

		"Missing array with 'required' tag does not bind": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"tspendpolicy":   {},
				"treasurypolicy": {}
			}`),
			expectedErr: "Key: 'SetVoteChoicesRequest.VoteChoices' Error:Field validation for 'VoteChoices' failed on the 'required' tag",
		},

		"Missing array with 'max' tag binds without error": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    {},
				"treasurypolicy": {}
			}`),
			expectedErr: "",
		},

		"Null array with 'required' tag does not bind": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    null,
				"tspendpolicy":   {},
				"treasurypolicy": {}
			}`),
			expectedErr: "Key: 'SetVoteChoicesRequest.VoteChoices' Error:Field validation for 'VoteChoices' failed on the 'required' tag",
		},

		"Null array with 'max' tag binds without error": {
			req: []byte(`{
				"timestamp":      12345,
				"tickethash":     "hash",
				"votechoices":    {},
				"tspendpolicy":   null,
				"treasurypolicy": {}
			}`),
			expectedErr: "",
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			err := binding.JSON.BindBody(test.req, &types.SetVoteChoicesRequest{})
			if test.expectedErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if err.Error() != test.expectedErr {
					t.Fatalf("incorrect error, got %q expected %q",
						err.Error(), test.expectedErr)
				}
			}

		})
	}

}
