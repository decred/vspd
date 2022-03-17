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

# run linter
golangci-lint run
