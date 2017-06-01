package client

func explainBillUsageToOwner(c *Client, v bool) string {
	if v {
		return "bills usage to the command's owner"
	} else {
		return "bills usage to **you**"
	}
}

func explainNeedsEscrow(c *Client, v bool) string {
	if v {
		return "uses the number after the command name as an escrow amount"
	} else {
		return "does not need to escrow any funds from you"
	}
}
