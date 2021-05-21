#!/bin/bash
#
# Copyright (c) 2020 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# usage:
# ./run_tests.sh

set -ex

go version

# run `go mod download` and `go mod tidy` and fail if the git status of
# go.mod and/or go.sum changes
MOD_STATUS=$(git status --porcelain go.mod go.sum)
go mod download
go mod tidy
UPDATED_MOD_STATUS=$(git status --porcelain go.mod go.sum)
if [ "$UPDATED_MOD_STATUS" != "$MOD_STATUS" ]; then
    echo "Running `go mod tidy` modified go.mod and/or go.sum"
    exit 1
fi

# run tests
env GORACE="halt_on_error=1" go test -race ./...

# set output format for linter
if [[ -v CI ]]; then
    OUT_FORMAT="github-actions"
else
    OUT_FORMAT="colored-line-number"
fi

# run linter
golangci-lint run --disable-all --deadline=10m \
  --out-format=$OUT_FORMAT \
  --enable=gofmt \
  --enable=revive \
  --enable=govet \
  --enable=gosimple \
  --enable=unconvert \
  --enable=ineffassign \
  --enable=structcheck \
  --enable=goimports \
  --enable=misspell \
  --enable=unparam \
  --enable=deadcode \
  --enable=unused \
  --enable=errcheck \
  --enable=asciicheck
  