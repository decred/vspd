// Copyright (c) 2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
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
func passwordHashPrompt(ctx context.Context, prompt string) ([]byte, error) {
	var passBytes []byte
	var err error

	// Ensure passBytes is not empty.
	for len(passBytes) == 0 {
		passBytes, err = passwordPrompt(ctx, prompt)
		if err != nil {
			return nil, err
		}
	}

	authHash := sha256.Sum256(passBytes)
	// Zero password bytes.
	clearBytes(passBytes)
	return authHash[:], nil
}

// readPassHashFromFile reads admin password hash from provided file.
func readPassHashFromFile(passwordDir string) ([]byte, error) {
	passwordFile, err := os.Open(passwordDir)
	if err != nil {
		return nil, err
	}
	defer passwordFile.Close()

	reader := bufio.NewReader(passwordFile)
	adminAuthHash, _, err := reader.ReadLine()
	if err != nil {
		return nil, err
	}

	return adminAuthHash, nil
}

// createPassHashFile prompts user for password,
// hashes the provided password and saves the hashed password to a file.
func createPassHashFile(ctx context.Context, passwordDir string) ([]byte, error) {
	adminAuthHash, err := passwordHashPrompt(ctx, "Enter admin Password:")
	if err != nil {
		return nil, err
	}
	passwordFile, err := os.Create(passwordDir)
	if err != nil {
		return nil, err
	}
	defer passwordFile.Close()
	// Length of byte is ignored
	_, err = passwordFile.Write(adminAuthHash)
	if err != nil {
		return nil, err
	}
	return adminAuthHash, nil
}
