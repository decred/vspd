package webapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"
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

func indentJSON(input string) template.HTML {
	var indented bytes.Buffer
	err := json.Indent(&indented, []byte(input), "<br/>", "&nbsp;&nbsp;&nbsp;&nbsp;")
	if err != nil {
		log.Errorf("Failed to indent JSON: %w", err)
		return template.HTML(input)
	}

	return template.HTML(indented.String())
}
