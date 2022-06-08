package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/decred/dcrd/wire"
)

type blockConnectedHandler struct {
	blockConnected chan *wire.BlockHeader
}

// Notify is called every time a notification is received from dcrd client.
// A wsrpc.Client will never call Notify concurrently. Notify should not return
// an error because that will cause the client to close and no further
// notifications will be received until a new connection is established.
func (n *blockConnectedHandler) Notify(method string, msg json.RawMessage) error {
	if method != "blockconnected" {
		return nil
	}

	header, err := parseBlockConnected(msg)
	if err != nil {
		log.Errorf("Failed to parse dcrd block notification: %v", err)
		return nil
	}

	n.blockConnected <- header

	return nil
}

func (n *blockConnectedHandler) Close() error {
	return nil
}

// parseBlockConnected extracts the block header from a
// blockconnected JSON-RPC notification.
func parseBlockConnected(msg json.RawMessage) (*wire.BlockHeader, error) {
	var notif []string
	err := json.Unmarshal(msg, &notif)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}

	if len(notif) == 0 {
		return nil, errors.New("notification is empty")
	}

	var header wire.BlockHeader
	err = header.Deserialize(hex.NewDecoder(bytes.NewReader([]byte(notif[0]))))
	if err != nil {
		return nil, fmt.Errorf("error creating block header from bytes: %w", err)
	}

	return &header, nil
}
