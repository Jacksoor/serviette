package client

func explainBillUsageToOwner(c *Client, v bool) string {
	if v {
		return "to script owner"
	} else {
		return "to executing user"
	}
}

func explainNeedsEscrow(c *Client, v bool) string {
	if v {
		return "uses number after command"
	} else {
		return "no escrow"
	}
}
