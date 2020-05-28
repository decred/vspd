package webapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jholdstock/vspd/rpc"
)

type ticketHashRequest struct {
	TicketHash string `json:"tickethash" binding:"required"`
}

// withDcrdClient middleware adds a dcrd client to the request
// context for downstream handlers to make use of.
func withDcrdClient() gin.HandlerFunc {
	return func(c *gin.Context) {
		dcrdConn, err := dcrdConnect()
		if err != nil {
			log.Errorf("dcrd connection error: %v", err)
			sendErrorResponse("dcrd RPC error", http.StatusInternalServerError, c)
			return
		}
		dcrdClient, err := rpc.DcrdClient(c, dcrdConn, cfg.NetParams)
		if err != nil {
			log.Errorf("dcrd client error: %v", err)
			sendErrorResponse("dcrd RPC error", http.StatusInternalServerError, c)
			return
		}

		c.Set("DcrdClient", dcrdClient)
	}
}

// withWalletClient middleware adds a voting wallet client to the request
// context for downstream handlers to make use of.
func withWalletClient() gin.HandlerFunc {
	return func(c *gin.Context) {
		walletClient := make([]*rpc.WalletRPC, len(walletConnect))
		for i := 0; i < len(walletConnect); i++ {
			walletConn, err := walletConnect[i]()
			if err != nil {
				log.Errorf("dcrwallet connection error: %v", err)
				sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
				return
			}
			walletClient[i], err = rpc.WalletClient(c, walletConn, cfg.NetParams)
			if err != nil {
				log.Errorf("dcrwallet client error: %v", err)
				sendErrorResponse("dcrwallet RPC error", http.StatusInternalServerError, c)
				return
			}
		}
		c.Set("WalletClient", walletClient)
	}
}

// vspAuth middleware reads the request body and extracts the ticket hash. The
// commitment address for the ticket is retrieved from the database if it is
// known, or it is retrieved from the chain if not.
// The middleware errors out if the VSP-Client-Signature header of the request
// does not contain the request body signed with the commitment address.
// Ticket information is added to the request context for downstream handlers to
// use.
func vspAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Read request bytes.
		reqBytes, err := c.GetRawData()
		if err != nil {
			log.Warnf("Error reading request from %s: %v", c.ClientIP(), err)
			sendErrorResponse(err.Error(), http.StatusBadRequest, c)
			return
		}

		// Add raw request to context for downstream handlers to use.
		c.Set("RawRequest", reqBytes)

		// Parse request and ensure there is a ticket hash included.
		var request ticketHashRequest
		if err := binding.JSON.BindBody(reqBytes, &request); err != nil {
			log.Warnf("Bad request from %s: %v", c.ClientIP(), err)
			sendErrorResponse(err.Error(), http.StatusBadRequest, c)
			return
		}
		hash := request.TicketHash

		// Check if this ticket already appears in the database.
		ticket, ticketFound, err := db.GetTicketByHash(hash)
		if err != nil {
			log.Errorf("GetTicketByHash error: %v", err)
			sendErrorResponse("database error", http.StatusInternalServerError, c)
			return
		}

		// If the ticket was found in the database we already know its commitment
		// address. Otherwise we need to get it from the chain.
		var commitmentAddress string
		if ticketFound {
			commitmentAddress = ticket.CommitmentAddress
		} else {
			dcrdClient := c.MustGet("DcrdClient").(*rpc.DcrdRPC)
			commitmentAddress, err = dcrdClient.GetTicketCommitmentAddress(hash, cfg.NetParams)
			if err != nil {
				log.Errorf("GetTicketCommitmentAddress error: %v", err)
				sendErrorResponse(err.Error(), http.StatusInternalServerError, c)
				return
			}
		}

		// Validate request signature to ensure ticket ownership.
		err = validateSignature(reqBytes, commitmentAddress, c)
		if err != nil {
			log.Warnf("Bad signature from %s: %v", c.ClientIP(), err)
			sendErrorResponse("bad signature", http.StatusBadRequest, c)
			return
		}

		// Add ticket information to context so downstream handlers don't need
		// to access the db for it.
		c.Set("Ticket", ticket)
		c.Set("KnownTicket", ticketFound)
		c.Set("CommitmentAddress", commitmentAddress)
	}

}
