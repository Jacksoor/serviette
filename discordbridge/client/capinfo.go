package client

import (
	"fmt"
)

func explainBillUsageToExecutingAccount(c *Client) string {
	return "bill usage to the executing account"
}

func explainWithdrawalLimit(c *Client, limit int64) string {
	return fmt.Sprintf("withdraw from your account up to limit of %d %s", limit, c.opts.CurrencyName)
}
