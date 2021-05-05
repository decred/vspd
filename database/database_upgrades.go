package database

import (
	"fmt"
)

const (
	// initialVersion is the version of a freshly created database which has had
	// no upgrades applied.
	initialVersion = 1

	// latestVersion is the latest version of the bolt database that is
	// understood by vspd. Databases with recorded versions higher than
	// this will fail to open (meaning any upgrades prevent reverting to older
	// software).
	latestVersion = initialVersion
)

// Upgrade will update the database to the latest known version.
func (vdb *VspDatabase) Upgrade(currentVersion uint32) error {
	if currentVersion == latestVersion {
		// No upgrades required.
		return nil
	}

	if currentVersion > latestVersion {
		// Database is too new.
		return fmt.Errorf("expected database version <= %d, got %d", latestVersion, currentVersion)
	}

	return nil
}
