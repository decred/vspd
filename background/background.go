package background

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"decred.org/dcrwallet/rpc/client/dcrd"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
	"github.com/jrick/wsrpc/v2"
)

var (
	ctx            context.Context
	db             *database.VspDatabase
	dcrdRPC        rpc.DcrdConnect
	walletRPC      rpc.WalletConnect
	netParams      *chaincfg.Params
	notifierClosed chan struct{}
	shutdownWg     *sync.WaitGroup
)

type NotificationHandler struct{}

const (
	// requiredConfs is the number of confirmations required to consider a
	// ticket purchase or a fee transaction to be final.
	requiredConfs = 6
)

// Notify is called every time a block notification is received from dcrd.
// Notify is never called concurrently. Notify should not return an error
// because that will cause the client to close and no further notifications will
// be received until a new connection is established.
func (n *NotificationHandler) Notify(method string, params json.RawMessage) error {
	if method != "blockconnected" {
		return nil
	}

	header, _, err := dcrd.BlockConnected(params)
	if err != nil {
		log.Errorf("Failed to parse dcrd block notification: %v", err)
		return nil
	}

	log.Debugf("Block notification %d (%s)", header.Height, header.BlockHash().String())

	blockConnected()

	return nil
}

// blockConnected is called once when vspd starts up, and once each time a
// blockconnected notification is received from dcrd.
func blockConnected() {

	funcName := "blockConnected"

	shutdownWg.Add(1)
	defer shutdownWg.Done()

	dcrdClient, err := dcrdRPC.Client(ctx, netParams)
	if err != nil {
		log.Errorf("%s: %v", funcName, err)
		return
	}

	// Step 1/3: Update the database with any tickets which now have 6+
	// confirmations.

	unconfirmed, err := db.GetUnconfirmedTickets()
	if err != nil {
		log.Errorf("%s: db.GetUnconfirmedTickets error: %v", funcName, err)
	}

	for _, ticket := range unconfirmed {
		tktTx, err := dcrdClient.GetRawTransaction(ticket.Hash)
		if err != nil {
			// ErrNoTxInfo here probably indicates a tx which was never mined
			// and has been removed from the mempool. For example, a ticket
			// purchase tx close to an sdiff change, or a ticket purchase tx
			// which expired. Remove it from the db.
			var e *wsrpc.Error
			if errors.As(err, &e) && e.Code == rpc.ErrNoTxInfo {
				log.Infof("%s: Removing unconfirmed ticket from db - no information available "+
					"about transaction (ticketHash=%s)", funcName, ticket.Hash)

				err = db.DeleteTicket(ticket)
				if err != nil {
					log.Errorf("%s: db.DeleteTicket error (ticketHash=%s): %v",
						funcName, ticket.Hash, err)
				}
			} else {
				log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
			}

			continue
		}

		if tktTx.Confirmations >= requiredConfs {
			ticket.Confirmed = true
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error (ticketHash=%s): %v", funcName, ticket.Hash, err)
				continue
			}

			log.Debugf("%s: Ticket confirmed (ticketHash=%s)", funcName, ticket.Hash)
		}
	}

	// Step 2/3: Broadcast fee tx for tickets which are confirmed.

	pending, err := db.GetPendingFees()
	if err != nil {
		log.Errorf("%s: db.GetPendingFees error: %v", funcName, err)
	}

	for _, ticket := range pending {
		err = dcrdClient.SendRawTransaction(ticket.FeeTxHex)
		if err != nil {
			log.Errorf("%s: dcrd.SendRawTransaction for fee tx failed (ticketHash=%s): %v",
				funcName, ticket.Hash, err)
			ticket.FeeTxStatus = database.FeeError
		} else {
			log.Debugf("%s: Fee tx broadcast for ticket (ticketHash=%s, feeHash=%s)",
				funcName, ticket.Hash, ticket.FeeTxHash)
			ticket.FeeTxStatus = database.FeeBroadcast
		}

		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("%s: db.UpdateTicket error (ticketHash=%s): %v", funcName, ticket.Hash, err)
		}
	}

	// Step 3/3: Add tickets with confirmed fees to voting wallets.

	unconfirmedFees, err := db.GetUnconfirmedFees()
	if err != nil {
		log.Errorf("%s: db.GetUnconfirmedFees error: %v", funcName, err)
		// If this fails, there is nothing more we can do. Return.
		return
	}

	// If there are no confirmed fees, there is nothing more to do. Return.
	if len(unconfirmedFees) == 0 {
		return
	}

	walletClients, failedConnections := walletRPC.Clients(ctx, netParams)
	if len(walletClients) == 0 {
		// If no wallet clients, there is nothing more we can do. Return.
		log.Errorf("%s: Could not connect to any wallets", funcName)
		return
	}
	if failedConnections > 0 {
		log.Errorf("%s: Failed to connect to %d wallet(s), proceeding with only %d",
			funcName, failedConnections, len(walletClients))
	}

	for _, ticket := range unconfirmedFees {
		feeTx, err := dcrdClient.GetRawTransaction(ticket.FeeTxHash)
		if err != nil {
			log.Errorf("%s: dcrd.GetRawTransaction for fee tx failed (feeTxHash=%s, ticketHash=%s): %v",
				funcName, ticket.FeeTxHash, ticket.Hash, err)
			continue
		}

		// If fee is confirmed, update the database and add ticket to voting
		// wallets.
		if feeTx.Confirmations >= requiredConfs {
			ticket.FeeTxStatus = database.FeeConfirmed
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("%s: db.UpdateTicket error (ticketHash=%s): %v", funcName, ticket.Hash, err)
				return
			}
			log.Debugf("%s: Fee tx confirmed (ticketHash=%s)", funcName, ticket.Hash)

			// Add ticket to the voting wallet.

			rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
			if err != nil {
				log.Errorf("%s: dcrd.GetRawTransaction for ticket failed (ticketHash=%s): %v",
					funcName, ticket.Hash, err)
				continue
			}
			for _, walletClient := range walletClients {
				err = walletClient.AddTicketForVoting(ticket.VotingWIF, rawTicket.BlockHash, rawTicket.Hex)
				if err != nil {
					log.Errorf("%s: dcrwallet.AddTicketForVoting error (wallet=%s, ticketHash=%s): %v",
						funcName, walletClient.String(), ticket.Hash, err)
					continue
				}

				// Update vote choices on voting wallets.
				for agenda, choice := range ticket.VoteChoices {
					err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
					if err != nil {
						log.Errorf("%s: dcrwallet.SetVoteChoice error (wallet=%s, ticketHash=%s): %v",
							funcName, walletClient.String(), ticket.Hash, err)
						continue
					}
				}
				log.Debugf("%s: Ticket added to voting wallet (wallet=%s, ticketHash=%s)",
					funcName, walletClient.String(), ticket.Hash)
			}
		}
	}
}

func (n *NotificationHandler) Close() error {
	close(notifierClosed)
	return nil
}

func connectNotifier(dcrdWithNotifs rpc.DcrdConnect) error {
	notifierClosed = make(chan struct{})

	dcrdClient, err := dcrdWithNotifs.Client(ctx, netParams)
	if err != nil {
		return err
	}

	err = dcrdClient.NotifyBlocks()
	if err != nil {
		return err
	}

	log.Info("Subscribed for dcrd block notifications")

	// Wait until context is done (vspd is shutting down), or until the
	// notifier is closed.
	select {
	case <-ctx.Done():
		return nil
	case <-notifierClosed:
		log.Warnf("dcrd notifier closed")
		return nil
	}
}

func Start(c context.Context, wg *sync.WaitGroup, vdb *database.VspDatabase, drpc rpc.DcrdConnect,
	dcrdWithNotif rpc.DcrdConnect, wrpc rpc.WalletConnect, p *chaincfg.Params) {

	ctx = c
	db = vdb
	dcrdRPC = drpc
	walletRPC = wrpc
	netParams = p
	shutdownWg = wg

	// Run the block connected handler now to catch up with any blocks mined
	// while vspd was shut down.
	blockConnected()

	// Loop forever attempting to create a connection to the dcrd server for
	// notifications.
	go func() {
		for {
			err := connectNotifier(dcrdWithNotif)
			if err != nil {
				log.Errorf("dcrd connect error: %v", err)
			}

			// If context is done (vspd is shutting down), return,
			// otherwise wait 15 seconds and try to reconnect.
			select {
			case <-ctx.Done():
				return
			case <-time.After(15 * time.Second):
			}

		}
	}()
}
