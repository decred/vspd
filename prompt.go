// Copyright (c) 2021 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"

	"golang.org/x/term"
)

type passwordReadResponse struct {
	password []byte
	err      error
}

// clearBytes zeroes the byte slice.
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// passwordPrompt prompts the user to enter a password. Password must not be an
// empty string.
func passwordPrompt(ctx context.Context, prompt string) ([]byte, error) {
	// Get the initial state of the terminal.
	initialTermState, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}

	passwordReadChan := make(chan passwordReadResponse, 1)

	go func() {
		fmt.Print(prompt)
		pass, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		passwordReadChan <- passwordReadResponse{
			password: pass,
			err:      err,
		}
	}()

	select {
	case <-ctx.Done():
		_ = term.Restore(int(os.Stdin.Fd()), initialTermState)
		return nil, ctx.Err()

	case res := <-passwordReadChan:
		if res.err != nil {
			return nil, res.err
		}
		return res.password, nil
	}
}

// passwordHashPrompt prompts the user to enter a password and returns its
// SHA256 hash. Password must not be an empty string.
func passwordHashPrompt(ctx context.Context, prompt string) ([sha256.Size]byte, error) {
	var passBytes []byte
	var err error
	var authSHA [sha256.Size]byte

	// Ensure passBytes is not empty.
	for len(passBytes) == 0 {
		passBytes, err = passwordPrompt(ctx, prompt)
		if err != nil {
			return authSHA, err
		}
	}

	authSHA = sha256.Sum256(passBytes)
	// Zero password bytes.
	clearBytes(passBytes)
	return authSHA, nil
}
