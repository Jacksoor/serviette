package client

import (
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

	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

var bankCommands map[string]command = map[string]command{
	"":        bankBalance,
	"bal":     bankBalance,
	"balance": bankBalance,

	"$":        bankAccounts,
	"accounts": bankAccounts,

	"pay": bankPay,

	"price": bankPrice,
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
		prettyBalances[i] = fmt.Sprintf("**%s:** %d %s", accountNames[i], balance, c.opts.CurrencyName)
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

	matches := discordMentionRegexp.FindStringSubmatch(parts[0])
	if len(matches) == 0 || matches[0] != parts[0] {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}
	targetID := matches[1]

	amount, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}

	sourceAccount, err := c.store.Account(ctx, m.Author.ID, checkingAccountName)
	if err != nil {
		return err
	}

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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, you don't have enough funds in your checking account to make that payment.", m.Author.ID))
			return nil
		}
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, you have been paid %d %s by <@%s>.", targetID, amount, c.opts.CurrencyName, m.Author.ID))
	return nil
}

func bankPrice(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	if rest == "" {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I didn't understand that. Please use `$price asset` to get the price of an asset.", m.Author.ID))
		return nil
	}

	resp, err := c.assetsClient.GetTypes(ctx, &assetspb.GetTypesRequest{})
	if err != nil {
		return err
	}

	var aliasDef *assetspb.TypeDefinition
	for _, def := range resp.Definition {
		if def.Name == rest {
			aliasDef = def
		}
	}

	if aliasDef != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, the current price of the **%s** asset (lease period: %s) is %d %s.", m.Author.ID, rest, durafmt.Parse(time.Duration(aliasDef.DurationSeconds)*time.Second).String(), aliasDef.Price, c.opts.CurrencyName))
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>, I don't have prices for the **%s** asset right now.", m.Author.ID, rest))
	}
	return nil
}
