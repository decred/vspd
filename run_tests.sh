#!/bin/bash
#
# Copyright (c) 2020-2021 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# usage:
# ./run_tests.sh

set -ex

go version

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
  