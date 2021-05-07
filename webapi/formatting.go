package webapi

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
