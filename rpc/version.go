// Copyright (c) 2020-2025 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package rpc

import (
	"fmt"

	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types/v4"
)

// minimumVersions contains the minimum expected binary and API versions for
// dcrd and dcrwallet.
var minimumVersions = map[string]semver{
	"dcrd":                {Major: 2, Minor: 1},
	"dcrdjsonrpcapi":      {Major: 8, Minor: 3},
	"dcrwallet":           {Major: 2, Minor: 1},
	"dcrwalletjsonrpcapi": {Major: 11, Minor: 0},
}

// checkVersion returns an error if the provided key in verMap does not have
// semver compatibility with the minimum expected versions.
func checkVersion(verMap map[string]dcrdtypes.VersionResult, key string) error {
	var actualV semver
	if ver, ok := verMap[key]; ok {
		actualV = semver{ver.Major, ver.Minor, ver.Patch}
	} else {
		return fmt.Errorf("version map missing key %q", key)
	}

	minimumV := minimumVersions[key]
	if !semverCompatible(minimumV, actualV) {
		return fmt.Errorf("incompatible %q version, expected %s got %s",
			key, minimumV, actualV)
	}
	return nil
}

type semver struct {
	Major uint32
	Minor uint32
	Patch uint32
}

func semverCompatible(required, actual semver) bool {
	switch {
	case required.Major != actual.Major:
		return false
	case required.Minor > actual.Minor:
		return false
	case required.Minor == actual.Minor && required.Patch > actual.Patch:
		return false
	default:
		return true
	}
}

func (s semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}
