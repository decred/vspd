#!/bin/bash
#
# Copyright (c) 2020-2022 The Decred developers
# Use of this source code is governed by an ISC
# license that can be found in the LICENSE file.
#
# usage:
# ./run_tests.sh

set -e

go version

# run tests on all modules
ROOTPATH=$(go list -m)
ROOTPATHPATTERN=$(echo $ROOTPATH | sed 's/\\/\\\\/g' | sed 's/\//\\\//g')
MODPATHS=$(go list -m all | grep "^$ROOTPATHPATTERN" | cut -d' ' -f1)
for module in $MODPATHS; do
  echo "==> ${module}"
  env GORACE="halt_on_error=1" go test -race ${module}/...

  # check linters
  MODNAME=$(echo $module | sed -E -e "s/^$ROOTPATHPATTERN//" \
    -e 's,^/,,' -e 's,/v[0-9]+$,,')
  if [ -z "$MODNAME" ]; then
    MODNAME=.
  fi

  # run commands in the module directory as a subshell
  (
    cd $MODNAME

    # run linter
    golangci-lint run
  )
done

echo "------------------------------------------"
echo "Tests completed successfully!"
