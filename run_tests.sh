#!/bin/bash

# usage:
# ./run_tests.sh

set -ex

go version

env GORACE="halt_on_error=1" go test -race ./...

if [[ -v CI ]]; then
    OUT_FORMAT="github-actions"
else
    OUT_FORMAT="colored-line-number"
fi

# some linters are commented until code is in a more stable state.
golangci-lint run --disable-all --deadline=10m \
  --out-format=$OUT_FORMAT \
  --enable=gofmt \
  --enable=golint \
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
  --enable=asciicheck
#  --enable=errcheck \