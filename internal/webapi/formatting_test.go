package webapi

import (
	"testing"

	"github.com/decred/slog"
)

func TestIndentJSON(t *testing.T) {
	t.Parallel()

	// Get the actual invokable func by passing a noop logger.
	indentJSONFunc := indentJSON(slog.Disabled)

	tests := map[string]struct {
		input    string
		expected string
	}{
		"nothing": {
			input:    "",
			expected: "",
		},
		"empty": {
			input:    "{}",
			expected: "{}",
		},
		"one line JSON": {
			input:    "{\"key\":\"value\"}",
			expected: "{\n    \"key\": \"value\"\n}",
		},
		"nested JSON": {
			input:    "{\"key\":{\"key2\":\"value\"}}",
			expected: "{\n    \"key\": {\n        \"key2\": \"value\"\n    }\n}",
		},
		"invalid JSON": {
			input:    "this is not valid json",
			expected: "this is not valid json",
		},
	}

	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()
			actual := indentJSONFunc(test.input)
			if actual != test.expected {
				t.Fatalf("expected %q, got %q", test.expected, actual)
			}
		})
	}
}
