package background

import (
	"context"
	"encoding/json"
	"time"

	"decred.org/dcrwallet/rpc/client/dcrd"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/vspd/database"
	"github.com/decred/vspd/rpc"
)

var (
	ctx            context.Context
	db             *database.VspDatabase
	dcrdRPC        rpc.DcrdConnect
	walletRPC      rpc.WalletConnect
	netParams      *chaincfg.Params
	notifierClosed chan struct{}
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

func blockConnected() {

	dcrdClient, err := dcrdRPC.Client(ctx, netParams)
	if err != nil {
		log.Error(err)
		return
	}

	// Step 1/3: Update the database with any tickets which now have 6+
	// confirmations.

	unconfirmed, err := db.GetUnconfirmedTickets()
	if err != nil {
		log.Errorf("GetUnconfirmedTickets error: %v", err)
	}

	for _, ticket := range unconfirmed {
		tktTx, err := dcrdClient.GetRawTransaction(ticket.Hash)
		if err != nil {
			log.Errorf("GetRawTransaction error: %v", err)
			continue
		}
		if tktTx.Confirmations >= requiredConfs {
			ticket.Confirmed = true
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("UpdateTicket error: %v", err)
				continue
			}

			log.Debugf("Ticket confirmed: ticketHash=%s", ticket.Hash)
		}
	}

	// Step 2/3: Broadcast fee tx for tickets which are confirmed.

	pending, err := db.GetPendingFees()
	if err != nil {
		log.Errorf("GetPendingFees error: %v", err)
	}

	for _, ticket := range pending {
		feeTxHash, err := dcrdClient.SendRawTransaction(ticket.FeeTxHex)
		if err != nil {
			// TODO: SendRawTransaction can return a "transcation already
			// exists" error, which isnt necessarily a problem here.
			log.Errorf("SendRawTransaction error: %v", err)
			continue
		}

		ticket.FeeTxHash = feeTxHash
		err = db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("UpdateTicket error: %v", err)
			continue
		}
		log.Debugf("Fee tx broadcast for ticket: ticketHash=%s, feeHash=%s", ticket.Hash, feeTxHash)
	}

	// Step 3/3: Add tickets with confirmed fees to voting wallets.

	unconfirmedFees, err := db.GetUnconfirmedFees()
	if err != nil {
		log.Errorf("GetUnconfirmedFees error: %v", err)
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
		log.Error("Could not connect to any wallets")
		return
	}
	if failedConnections > 0 {
		log.Errorf("Failed to connect to %d wallet(s), proceeding with only %d",
			failedConnections, len(walletClients))
	}

	for _, ticket := range unconfirmedFees {
		feeTx, err := dcrdClient.GetRawTransaction(ticket.FeeTxHash)
		if err != nil {
			log.Errorf("GetRawTransaction error: %v", err)
			continue
		}

		// If fee is confirmed, update the database and add ticket to voting
		// wallets.
		if feeTx.Confirmations >= requiredConfs {
			ticket.FeeConfirmed = true
			err = db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("UpdateTicket error: %v", err)
				return
			}
			log.Debugf("Fee tx confirmed for ticket: ticketHash=%s", ticket.Hash)

			// Add ticket to the voting wallet.

			rawTicket, err := dcrdClient.GetRawTransaction(ticket.Hash)
			if err != nil {
				log.Errorf("GetRawTransaction error: %v", err)
				continue
			}
			for _, walletClient := range walletClients {
				err = walletClient.ImportPrivKey(ticket.VotingWIF)
				if err != nil {
					log.Errorf("ImportPrivKey error on dcrwallet '%s': %v",
						walletClient.String(), err)
					continue
				}

				err = walletClient.AddTransaction(rawTicket.BlockHash, rawTicket.Hex)
				if err != nil {
					log.Errorf("AddTransaction error on dcrwallet '%s': %v",
						walletClient.String(), err)
					continue
				}

				// Update vote choices on voting wallets.
				for agenda, choice := range ticket.VoteChoices {
					err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
					if err != nil {
						log.Errorf("SetVoteChoice error on dcrwallet '%s': %v",
							walletClient.String(), err)
						continue
					}
				}
				log.Debugf("Ticket added to voting wallet '%s': ticketHash=%s",
					walletClient.String(), ticket.Hash)
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
		return ctx.Err()
	case <-notifierClosed:
		return nil
	}
}

func Start(c context.Context, vdb *database.VspDatabase, drpc rpc.DcrdConnect,
	dcrdWithNotif rpc.DcrdConnect, wrpc rpc.WalletConnect, p *chaincfg.Params) {

	ctx = c
	db = vdb
	dcrdRPC = drpc
	walletRPC = wrpc
	netParams = p

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

				// If context is done (vspd is shutting down), return,
				// otherwise wait 15 seconds and try to reconnect.
				select {
				case <-ctx.Done():
					return
				case <-time.After(15 * time.Second):
				}
			}

		}
	}()
}
