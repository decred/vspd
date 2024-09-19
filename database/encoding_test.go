// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"reflect"
	"testing"
)

func TestBytesToStringMap(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name      string
		input     []byte
		expect    map[string]string
		expectErr bool
	}{
		{
			name:      "Empty map on nil bytes",
			input:     nil,
			expect:    map[string]string{},
			expectErr: false,
		},
		{
			name:      "Empty map on empty json map",
			input:     []byte("{}"),
			expect:    map[string]string{},
			expectErr: false,
		},
		{
			name:      "Empty map on null",
			input:     []byte("null"),
			expect:    map[string]string{},
			expectErr: false,
		},
		{
			name:      "Correct values with valid json",
			input:     []byte("{\"key\":\"value\"}"),
			expect:    map[string]string{"key": "value"},
			expectErr: false,
		},
		{
			name:      "Error on no bytes",
			input:     []byte(""),
			expect:    nil,
			expectErr: true,
		},
		{
			name:      "Error on invalid json",
			input:     []byte("invalid json"),
			expect:    nil,
			expectErr: true,
		},
		{
			name:      "Error on non-map json",
			input:     []byte("[\"not a map\"]"),
			expect:    nil,
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := bytesToStringMap(test.input)
			if !reflect.DeepEqual(test.expect, result) {
				t.Fatalf("expected %v, got %v", test.expect, result)
			}
			if test.expectErr != (err != nil) {
				t.Fatalf("expected err=%t, got %v", test.expectErr, err)
			}
		})
	}
}
