package webapi

import (
	"fmt"
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
	return time.Unix(t, 0).Format("2 Jan 2006 15:04:05")
}