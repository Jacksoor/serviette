package client

func explainBillUsageToOwner(c *Client, v bool) string {
	if v {
		return "to owner"
	} else {
		return "to **you**"
	}
}

func explainNeedsEscrow(c *Client, v bool) string {
	if v {
		return "uses number after command"
	} else {
		return "no escrow"
	}
}
