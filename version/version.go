// Copyright (c) 2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package version

import (
	"bytes"
	"fmt"
	"strings"
)

// semverAlphabet is an alphabet of all characters allowed in semver prerelease
// identifiers, and the . separator.
const semverAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-."

// Constants defining the application version number.
const (
	Major = 1
	Minor = 0
	Patch = 0
)

// PreRelease contains the prerelease name of the application. It is a variable
// so it can be modified at link time (e.g.
// `-ldflags "-X decred.org/vspd/version.PreRelease=rc1"`).
// It must only contain characters from the semantic version alphabet.
var PreRelease = "pre"

// String returns the application version as a properly formed string per the
// semantic versioning 2.0.0 spec (https://semver.org/).
func String() string {
	// Start with the major, minor, and path versions.
	version := fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)

	// Append pre-release version if there is one. The hyphen called for
	// by the semantic versioning spec is automatically appended and should
	// not be contained in the pre-release string. The pre-release version
	// is not appended if it contains invalid characters.
	preRelease := normalizeVerString(PreRelease)
	if preRelease != "" {
		version = version + "-" + preRelease
	}

	return version
}

// normalizeVerString returns the passed string stripped of all characters which
// are not valid according to the semantic versioning guidelines for pre-release
// version and build metadata strings. In particular they MUST only contain
// characters in semanticAlphabet.
func normalizeVerString(str string) string {
	var buf bytes.Buffer
	for _, r := range str {
		if strings.ContainsRune(semverAlphabet, r) {
			_, err := buf.WriteRune(r)
			// Writing to a bytes.Buffer panics on OOM, and all
			// errors are unexpected.
			if err != nil {
				panic(err)
			}
		}
	}
	return buf.String()
}
