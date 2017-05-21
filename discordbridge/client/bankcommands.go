package client

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

var bankCommands map[string]command = map[string]command{
	"":        bankBalance,
	"bal":     bankBalance,
	"balance": bankBalance,

	"$":       bankAccount,
	"account": bankAccount,

	"pay": bankPay,

	"help": bankHelp,
}

var discordMentionRegexp = regexp.MustCompile(`<@!?(\d+)>`)

func bankBalance(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	var targetID string
	if rest == "" {
		targetID = m.Author.ID
	} else {
		matches := discordMentionRegexp.FindStringSubmatch(rest)
		if len(matches) == 0 || matches[0] != rest {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `$ @nickname` to ask for someone's balance.", m.Author.ID))
			return nil
		}

		targetID = matches[1]
	}

	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(targetID),
	})
	if err != nil {
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{resolveResp.AccountHandle},
	})
	if err != nil {
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s> has %d %s.", targetID, resp.Balance[0], c.opts.CurrencyName))
	return nil
}

func bankAccount(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	var targetID string
	if rest == "" {
		targetID = m.Author.ID
	} else {
		matches := discordMentionRegexp.FindStringSubmatch(rest)
		if len(matches) == 0 || matches[0] != rest {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `$ @nickname` to ask for someone's balance.", m.Author.ID))
			return nil
		}

		targetID = matches[1]
	}

	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(targetID),
	})
	if err != nil {
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{resolveResp.AccountHandle},
	})
	if err != nil {
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>'s account handle is `%s` and has %d %s.", targetID, base64.RawURLEncoding.EncodeToString(resolveResp.AccountHandle), resp.Balance[0], c.opts.CurrencyName))
	return nil
}

func bankPay(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}

	amount, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}

	sourceResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	matches := discordMentionRegexp.FindStringSubmatch(parts[0])
	if len(matches) == 0 || matches[0] != parts[0] {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `$pay @nickname amount` to pay someone.", m.Author.ID))
		return nil
	}
	targetID := matches[1]
	if err := c.ensureAccount(ctx, targetID); err != nil {
		return err
	}

	targetResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(targetID),
	})
	if err != nil {
		return err
	}

	_, err = c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceResolveResp.AccountHandle,
		SourceAccountKey:    sourceResolveResp.AccountKey,
		TargetAccountHandle: targetResolveResp.AccountHandle,
		Amount:              amount,
	})
	if err != nil {
		if grpc.Code(err) == codes.FailedPrecondition {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, you don't have enough funds in your account to make that payment.", m.Author.ID))
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, that's not an amount you can transfer.", m.Author.ID))
			return nil
		}
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>, you have been paid %d %s by <@!%s>.", targetID, amount, c.opts.CurrencyName, m.Author.ID))
	return nil
}

func bankHelp(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error {
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(`Hi <@!%s>, I understand the following commands:

`+"`"+`$balance [@username]`+"`"+`
**Also available as:** `+"`"+`$`+"`"+`, `+"`"+`$bal`+"`"+`
Get a user's balance. Leave out the username to get your own balance.

`+"`"+`$account [@username]`+"`"+`
**Also available as:** `+"`"+`$$`+"`"+`
Get a user's account information. Leave out the username to get your own accounts.

`+"`"+`$pay @username amount`+"`"+`
Pay a user from your account into their account.
`, m.Author.ID))
	return nil
}
