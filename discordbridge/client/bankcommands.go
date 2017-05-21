package client

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"
	"github.com/hako/durafmt"

	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	namespb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
)

var bankCommands map[string]command = map[string]command{
	"":        bankBalance,
	"bal":     bankBalance,
	"balance": bankBalance,

	"$":        bankAccounts,
	"accounts": bankAccounts,

	"pay": bankPay,

	"prices": bankPrices,

	"help": bankHelp,
}

var discordMentionRegexp = regexp.MustCompile(`<@(\d+)>`)

func bankBalance(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	var targetID string
	if rest == "" {
		targetID = m.Author.ID
	} else {
		matches := discordMentionRegexp.FindStringSubmatch(rest)
		if len(matches) == 0 || matches[0] != rest {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$ @nickname` to ask for someone's balance.", m.Author.ID))
			return nil
		}

		targetID = matches[1]
	}

	accounts, err := c.store.Accounts(ctx, targetID)
	if err != nil {
		return err
	}

	accountHandles := make([][]byte, len(accounts))
	i := 0
	for _, account := range accounts {
		accountHandles[i] = account.Handle
		i++
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandles,
	})
	if err != nil {
		return err
	}

	var balance int64
	for _, subbalance := range resp.Balance {
		balance += subbalance
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> has %d %s.", targetID, balance, c.opts.CurrencyName))
	return nil
}

func bankAccounts(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	var targetID string
	if rest == "" {
		targetID = m.Author.ID
	} else {
		matches := discordMentionRegexp.FindStringSubmatch(rest)
		if len(matches) == 0 || matches[0] != rest {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$ @nickname` to ask for someone's balance.", m.Author.ID))
			return nil
		}

		targetID = matches[1]
	}

	accounts, err := c.store.Accounts(ctx, targetID)
	if err != nil {
		return err
	}

	accountNames := make([]string, len(accounts))
	accountHandles := make([][]byte, len(accounts))

	i := 0
	for name, account := range accounts {
		accountNames[i] = name
		accountHandles[i] = account.Handle
		i++
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandles,
	})
	if err != nil {
		return err
	}

	prettyBalances := make([]string, len(resp.Balance))

	for i, balance := range resp.Balance {
		prettyBalances[i] = fmt.Sprintf("**%s (account handle: `%s`):** %d %s", accountNames[i], base64.StdEncoding.EncodeToString(accountHandles[i]), balance, c.opts.CurrencyName)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>'s account balances:\n\n%s", targetID, strings.Join(prettyBalances, "\n")))
	return nil
}

func bankPay(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}

	amount, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}

	sourceAccount, err := c.store.Account(ctx, m.Author.ID, checkingAccountName)
	if err != nil {
		return err
	}

	matches := discordMentionRegexp.FindStringSubmatch(parts[0])
	if len(matches) == 0 || matches[0] != parts[0] {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}
	targetID := matches[1]
	if err := c.ensureAccount(ctx, targetID); err != nil {
		return err
	}

	targetAccount, err := c.store.Account(ctx, targetID, checkingAccountName)
	if err != nil {
		return err
	}

	_, err = c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceAccount.Handle,
		SourceAccountKey:    sourceAccount.Key,
		TargetAccountHandle: targetAccount.Handle,
		Amount:              amount,
	})
	if err != nil {
		if grpc.Code(err) == codes.FailedPrecondition {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, you don't have enough funds in your checking account to make that payment.", m.Author.ID))
			return nil
		}
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, you have been paid %d %s by <@%s>.", targetID, amount, c.opts.CurrencyName, m.Author.ID))
	return nil
}

func bankPrices(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	if rest != "" {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$prices` to get the prices of names.", m.Author.ID))
		return nil
	}

	resp, err := c.namesClient.GetTypes(ctx, &namespb.GetTypesRequest{})
	if err != nil {
		return err
	}

	prettyPrices := make([]string, len(resp.Definition))
	for i, def := range resp.Definition {
		prettyPrices[i] = fmt.Sprintf("**`%s` (lease period: %s):** %d %s", def.Name, durafmt.Parse(time.Duration(def.DurationSeconds)*time.Second).String(), def.Price, c.opts.CurrencyName)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, here are the current prices of names:\n\n%s", m.Author.ID, strings.Join(prettyPrices, "\n")))
	return nil
}

func bankHelp(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(`Hi <@%s>, I understand the following commands:

`+"`"+`$balance [@username]`+"`"+`
**Also available as:** `+"`"+`$`+"`"+`, `+"`"+`$bal`+"`"+`
Get a user's balance. Leave out the username to get your own balance.

`+"`"+`$accounts [@username]`+"`"+`
**Also available as:** `+"`"+`$$`+"`"+`
Get a user's accounts. Leave out the username to get your own accounts.

`+"`"+`$pay @username amount`+"`"+`
Pay a user from your checking account into their checking account.

`+"`"+`$prices`+"`"+`
Get the prices of all name types.
`, m.Author.ID))
	return nil
}
