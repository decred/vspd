package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrutil/v4"
	"github.com/decred/slog"
)

func addressURL(blockExplorerURL string) func(string) string {
	return func(addr string) string {
		return fmt.Sprintf("%s/address/%s", blockExplorerURL, addr)
	}
}

func txURL(blockExplorerURL string) func(string) string {
	return func(txID string) string {
		return fmt.Sprintf("%s/tx/%s", blockExplorerURL, txID)
	}
}

func blockURL(blockExplorerURL string) func(int64) string {
	return func(height int64) string {
		return fmt.Sprintf("%s/block/%d", blockExplorerURL, height)
	}
}

func dateTime(t int64) string {
	return time.Unix(t, 0).Format("2 Jan 2006 15:04:05 MST")
}

func stripWss(input string) string {
	input = strings.ReplaceAll(input, "wss://", "")
	input = strings.ReplaceAll(input, "/ws", "")
	return input
}

// indentJSON returns a func which uses whitespace to format a provided JSON
// string. If the parameter is invalid JSON, an error will be logged and the
// param will be returned unaltered.
func indentJSON(log slog.Logger) func(string) string {
	return func(input string) string {
		var indented bytes.Buffer
		err := json.Indent(&indented, []byte(input), "", "    ")
		if err != nil {
			log.Errorf("Failed to indent JSON: %w", err)
			return input
		}

		return indented.String()
	}
}

func atomsToDCR(atoms int64) string {
	return dcrutil.Amount(atoms).String()
}

func float32ToPercent(input float32) string {
	return fmt.Sprintf("%.2f%%", input*100)
}
