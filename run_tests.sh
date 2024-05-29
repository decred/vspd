#!/usr/bin/env bash
#
# Copyright (c) 2020-2024 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Usage:
#   ./run_tests.sh

set -e

go version

# This list needs to be updated if new submodules are added to the vspd repo.
submodules="client types"

# Test main module.
echo "==> test main module"
GORACE="halt_on_error=1" go test -race ./...

# Test all submodules in a subshell.
for module in $submodules
do
  echo "==> test ${module}"
  (
    cd $module
    GORACE="halt_on_error=1" go test -race .
  )
done

# Lint main module.
echo "==> lint main module"
golangci-lint run

# Lint all submodules in a subshell.
for module in $submodules
do
  echo "==> lint ${module}"
  (
    cd $module
    golangci-lint run
  )
done

echo "-----------------------------"
echo "Tests completed successfully!"
