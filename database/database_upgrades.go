package database

import (
	"fmt"

	bolt "go.etcd.io/bbolt"
)

const (
	// initialVersion is the version of a freshly created database which has had
	// no upgrades applied.
	initialVersion = 1

	// removeOldFeeTxVersion deletes any raw fee transactions which remain in
	// the database after already having been confirmed on-chain. There is no
	// need to keep these, and they take up a lot of space.
	removeOldFeeTxVersion = 2

	// latestVersion is the latest version of the database that is understood by
	// vspd. Databases with recorded versions higher than this will fail to open
	// (meaning any upgrades prevent reverting to older software).
	latestVersion = removeOldFeeTxVersion
)

// upgrades maps between old database versions and the upgrade function to
// upgrade the database to the next version.
var upgrades = []func(tx *bolt.DB) error{
	initialVersion: removeOldFeeTxUpgrade,
}

// Upgrade will update the database to the latest known version.
func (vdb *VspDatabase) Upgrade(currentVersion uint32) error {
	if currentVersion == latestVersion {
		// No upgrades required.
		return nil
	}

	if currentVersion > latestVersion {
		// Database is too new.
		return fmt.Errorf("expected database version <= %d, got %d",
			latestVersion, currentVersion)
	}

	// Execute all necessary upgrades in order.
	for _, upgrade := range upgrades[currentVersion:] {
		err := upgrade(vdb.db)
		if err != nil {
			return err
		}
	}

	return nil
}
