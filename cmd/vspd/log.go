// Copyright (c) 2020-2022 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/slog"
	"github.com/jrick/logrotate/rotator"
)

// logWriter implements an io.Writer that outputs to both standard output and
// the write-end pipe of an initialized log rotator.
type logWriter struct {
	rotator *rotator.Rotator
}

func (lw logWriter) Write(p []byte) (n int, err error) {
	os.Stdout.Write(p)
	return lw.rotator.Write(p)
}

func newLogBackend(logDir string, appName string, maxLogSize int64, logsToKeep int) (*slog.Backend, error) {
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logFileName := fmt.Sprintf("%s.log", appName)
	logFilePath := filepath.Join(logDir, logFileName)

	r, err := rotator.New(logFilePath, maxLogSize*1024, false, logsToKeep)
	if err != nil {
		return nil, fmt.Errorf("failed to create log rotator: %w", err)
	}

	return slog.NewBackend(logWriter{r}), nil
}
