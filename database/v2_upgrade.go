package database

import (
	"encoding/json"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

func removeOldFeeTxUpgrade(db *bolt.DB) error {
	log.Infof("Upgrading database to version %d", removeOldFeeTxVersion)

	// Run the upgrade in a single database transaction so it can be safely
	// rolled back if an error is encountered.
	err := db.Update(func(tx *bolt.Tx) error {
		vspBkt := tx.Bucket(vspBktK)
		ticketBkt := vspBkt.Bucket(ticketBktK)

		count := 0
		err := ticketBkt.ForEach(func(k, v []byte) error {
			// Deserialize the old ticket.
			var ticket Ticket
			err := json.Unmarshal(v, &ticket)
			if err != nil {
				return fmt.Errorf("could not unmarshal ticket: %w", err)
			}

			// Remove the fee tx hex if the tx is already confirmed.
			if ticket.FeeTxStatus == FeeConfirmed {
				count++
				ticket.FeeTxHex = ""
				ticketBytes, err := json.Marshal(ticket)
				if err != nil {
					return fmt.Errorf("could not marshal ticket: %w", err)
				}

				err = ticketBkt.Put(k, ticketBytes)
				if err != nil {
					return fmt.Errorf("could not put updated ticket: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			return err
		}

		log.Infof("Dropped %d unnecessary raw transactions", count)

		// Update database version.
		err = vspBkt.Put(versionK, uint32ToBytes(removeOldFeeTxVersion))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	log.Info("Upgrade completed")
	return nil
}
