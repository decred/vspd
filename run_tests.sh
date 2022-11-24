#!/bin/bash
#
# Copyright (c) 2020-2022 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# Usage:
#   ./run_tests.sh

set -e

go version

# Run tests on root module and all submodules.
echo "==> test all modules"
ROOTPKG="github.com/decred/vspd"
GORACE="halt_on_error=1" go test -race $ROOTPKG/...

# Find submodules.
ROOTPKGPATTERN=$(echo $ROOTPKG | sed 's/\\/\\\\/g' | sed 's/\//\\\//g')
MODPATHS=$(go list -m all | grep "^$ROOTPKGPATTERN" | cut -d' ' -f1)

for module in $MODPATHS; do

  echo "==> lint ${module}"

  # Get the path of the module.
  MODNAME=$(echo $module | sed -E -e "s/^$ROOTPKGPATTERN//" \
    -e 's,^/,,' -e 's,/v[0-9]+$,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  # Run commands in the module directory as a subshell.
  (
    cd $MODNAME

    golangci-lint run
  )
done

echo "-----------------------------"
echo "Tests completed successfully!"
