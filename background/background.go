package background

import (
	"context"
	"encoding/json"
	"time"

	"decred.org/dcrwallet/rpc/client/dcrd"
	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/jholdstock/dcrvsp/database"
	"github.com/jholdstock/dcrvsp/rpc"
)

type NotificationHandler struct {
	Ctx           context.Context
	Db            *database.VspDatabase
	WalletConnect rpc.Connect
	NetParams     *chaincfg.Params
	closed        chan struct{}
	dcrdClient    *rpc.DcrdRPC
}

// The number of confirmations required to consider a ticket purchase or a fee
// transaction to be final.
const (
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

	// Step 1/3: Update the database with any tickets which now have 6+
	// confirmations.

	unconfirmed, err := n.Db.GetUnconfirmedTickets()
	if err != nil {
		log.Errorf("GetUnconfirmedTickets error: %v", err)
	}

	for _, ticket := range unconfirmed {
		tktTx, err := n.dcrdClient.GetRawTransaction(ticket.Hash)
		if err != nil {
			log.Errorf("GetRawTransaction error: %v", err)
			continue
		}
		if tktTx.Confirmations >= requiredConfs {
			ticket.Confirmed = true
			err = n.Db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("UpdateTicket error: %v", err)
				continue
			}

			log.Debugf("Ticket confirmed: ticketHash=%s", ticket.Hash)
		}
	}

	// Step 2/3: Broadcast fee tx for tickets which are confirmed.

	pending, err := n.Db.GetPendingFees()
	if err != nil {
		log.Errorf("GetPendingFees error: %v", err)
	}

	for _, ticket := range pending {
		feeTxHash, err := n.dcrdClient.SendRawTransaction(ticket.FeeTxHex)
		if err != nil {
			// TODO: SendRawTransaction can return a "transcation already
			// exists" error, which isnt necessarily a problem here.
			log.Errorf("SendRawTransaction error: %v", err)
			continue
		}

		ticket.FeeTxHash = feeTxHash
		err = n.Db.UpdateTicket(ticket)
		if err != nil {
			log.Errorf("UpdateTicket error: %v", err)
			continue
		}
		log.Debugf("Fee tx broadcast for ticket: ticketHash=%s, feeHash=%s", ticket.Hash, feeTxHash)
	}

	// Step 3/3: Add tickets with confirmed fees to voting wallets.

	unconfirmedFees, err := n.Db.GetUnconfirmedFees()
	if err != nil {
		log.Errorf("GetUnconfirmedFees error: %v", err)
		// If this fails, there is nothing more we can do. Return.
		return nil
	}

	// If there are no confirmed fees, there is nothing more to do. Return.
	if len(unconfirmedFees) == 0 {
		return nil
	}

	var walletClient *rpc.WalletRPC
	walletConn, err := n.WalletConnect()
	if err != nil {
		log.Errorf("dcrwallet connection error: %v", err)
		// If this fails, there is nothing more we can do. Return.
		return nil
	}
	walletClient, err = rpc.WalletClient(n.Ctx, walletConn, n.NetParams)
	if err != nil {
		log.Errorf("dcrwallet client error: %v", err)
		// If this fails, there is nothing more we can do. Return.
		return nil
	}

	for _, ticket := range unconfirmedFees {
		feeTx, err := n.dcrdClient.GetRawTransaction(ticket.FeeTxHash)
		if err != nil {
			log.Errorf("GetRawTransaction error: %v", err)
			continue
		}

		// If fee is confirmed, update the database and add ticket to voting
		// wallets.
		if feeTx.Confirmations >= requiredConfs {
			ticket.FeeConfirmed = true
			err = n.Db.UpdateTicket(ticket)
			if err != nil {
				log.Errorf("UpdateTicket error: %v", err)
				return nil
			}
			log.Debugf("Fee tx confirmed for ticket: ticketHash=%s", ticket.Hash)

			// Add ticket to the voting wallet.

			rawTicket, err := n.dcrdClient.GetRawTransaction(ticket.Hash)
			if err != nil {
				log.Errorf("GetRawTransaction error: %v", err)
				continue
			}
			err = walletClient.AddTransaction(rawTicket.BlockHash, rawTicket.Hex)
			if err != nil {
				log.Errorf("AddTransaction error: %v", err)
				continue
			}
			err = walletClient.ImportPrivKey(ticket.VotingWIF)
			if err != nil {
				log.Errorf("ImportPrivKey error: %v", err)
				continue
			}

			// Update vote choices on voting wallets.
			for agenda, choice := range ticket.VoteChoices {
				err = walletClient.SetVoteChoice(agenda, choice, ticket.Hash)
				if err != nil {
					log.Errorf("SetVoteChoice error: %v", err)
					continue
				}
			}
			log.Debugf("Ticket added to voting wallet: ticketHash=%s", ticket.Hash)
		}
	}

	return nil
}

func (n *NotificationHandler) Close() error {
	close(n.closed)
	return nil
}

func (n *NotificationHandler) connect(dcrdConnect rpc.Connect) error {
	dcrdConn, err := dcrdConnect()
	if err != nil {
		return err
	}
	n.dcrdClient, err = rpc.DcrdClient(n.Ctx, dcrdConn, n.NetParams)
	if err != nil {
		return err
	}

	err = n.dcrdClient.NotifyBlocks()
	if err != nil {
		return err
	}

	log.Info("Subscribed for dcrd block notifications")

	// Wait until context is done (dcrvsp is shutting down), or until the
	// notifier is closed.
	select {
	case <-n.Ctx.Done():
		return n.Ctx.Err()
	case <-n.closed:
		return nil
	}
}

func Start(n *NotificationHandler, dcrdConnect rpc.Connect) {

	// Loop forever attempting to create a connection to the dcrd server.
	go func() {
		for {
			n.closed = make(chan struct{})

			err := n.connect(dcrdConnect)
			if err != nil {
				log.Errorf("dcrd connect error: %v", err)

				// If context is done (dcrvsp is shutting down), return,
				// otherwise wait 15 seconds and to reconnect.
				select {
				case <-n.Ctx.Done():
					return
				case <-time.After(15 * time.Second):
				}
			}

		}

	}()
}
