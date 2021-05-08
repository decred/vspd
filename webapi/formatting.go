package webapi

import "time"

func addressURL(blockExplorerURL string) func(string) string {
	return func(addr string) string {
		return blockExplorerURL + "/address/" + addr
	}
}

func txURL(blockExplorerURL string) func(string) string {
	return func(txID string) string {
		return blockExplorerURL + "/tx/" + txID
	}
}

func dateTime(t int64) string {
	return time.Unix(t, 0).Format("2 Jan 2006 15:04:05")
}
