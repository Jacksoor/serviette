package client

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func explainBillUsageToExecutingAccount(c *Client) string {
	return "bill usage to the executing account"
}

func explainWithdrawalLimit(c *Client, channel *discordgo.Channel, limit int64) string {
	return fmt.Sprintf("withdraw from your account up to limit of %d %s", limit, c.currencyName(channel.GuildID))
}
