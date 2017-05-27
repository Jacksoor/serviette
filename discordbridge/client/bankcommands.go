package client

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var bankCommands map[string]command = map[string]command{
	"":        bankBalance,
	"bal":     bankBalance,
	"balance": bankBalance,

	"$":       bankAccount,
	"account": bankAccount,

	"pay": bankPay,

	"cmd": bankCmd,

	"setcaps": bankSetcaps,

	"key": bankKey,

	"?":    bankHelp,
	"help": bankHelp,
}

var discordMentionRegexp = regexp.MustCompile(`<@!?(\d+)>`)

func bankBalance(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	target := rest

	if target == "" {
		target = fmt.Sprintf("<@!%s>", m.Author.ID)
	}

	accountHandle, err := resolveAccountTarget(ctx, c, target)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, %s doesn't have an account.", m.Author.ID, target))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, `%s` is neither a @mention nor an account handle.", m.Author.ID, target))
			return nil
		}
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{accountHandle},
	})
	if err != nil {
		return err
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s has %d %s.", target, resp.Balance[0], c.currencyName(channel.GuildID)))
	return nil
}

func bankAccount(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	target := rest

	if target == "" {
		target = fmt.Sprintf("<@!%s>", m.Author.ID)
	}

	accountHandle, err := resolveAccountTarget(ctx, c, target)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, %s doesn't have an account.", m.Author.ID, target))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, `%s` is neither a @mention nor an account handle.", m.Author.ID, target))
			return nil
		}
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{accountHandle},
	})
	if err != nil {
		return err
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s's account handle is `%s` and has %d %s.", target, base64.RawURLEncoding.EncodeToString(accountHandle), resp.Balance[0], c.currencyName(channel.GuildID)))
	return nil
}

func bankPay(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `%spay @mention/handle amount` to pay someone.", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	amount, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand the amount you wanted to pay. Please use `%spay @mention/handle amount` to pay someone.", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	sourceResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	target := parts[0]
	targetAccountHandle, err := resolveAccountTarget(ctx, c, target)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, %s doesn't have an account.", m.Author.ID, target))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, `%s` is neither a @mention nor an account handle.", m.Author.ID, target))
			return nil
		}
		return err
	}

	_, err = c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceResolveResp.AccountHandle,
		TargetAccountHandle: targetAccountHandle,
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

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("%s, you have been paid %d %s by <@!%s>.", target, amount, c.currencyName(channel.GuildID), m.Author.ID))
	return nil
}

func bankCmd(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	sourceResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	commandName := rest

	scriptAccountHandle, scriptName, aliased, err := resolveScriptName(ctx, c, commandName)
	if err != nil {
		if err == errNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I don't know what the `%s` command is.", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	getRequestedCapsResp, err := c.scriptsClient.GetRequestedCapabilities(ctx, &scriptspb.GetRequestedCapabilitiesRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, the owner of the `%s` command hasn't configured their command correctly.", m.Author.ID, commandName))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I couldn't find a command named `%s` owned by the account `%s`.", m.Author.ID, scriptName, base64.RawURLEncoding.EncodeToString(scriptAccountHandle)))
			}
			return nil
		}
		return err
	}

	prettyRequestedCaps := make([]string, 0)
	if getRequestedCapsResp.Capabilities.BillUsageToExecutingAccount {
		prettyRequestedCaps = append(prettyRequestedCaps, " - "+explainBillUsageToExecutingAccount(c))
	}

	if getRequestedCapsResp.Capabilities.WithdrawalLimit > 0 {
		prettyRequestedCaps = append(prettyRequestedCaps, " - "+explainWithdrawalLimit(c, channel, getRequestedCapsResp.Capabilities.WithdrawalLimit))
	}

	var prettyRequestedCapDetails string
	if len(prettyRequestedCaps) > 0 {
		prettyRequestedCapDetails = fmt.Sprintf("**This command requests your following capabilities:**\n\n%s", strings.Join(prettyRequestedCaps, "\n"))
	} else {
		prettyRequestedCapDetails = fmt.Sprintf("**This command requests none of your capabilities.**")
	}

	getAccountCapsResp, err := c.scriptsClient.GetAccountCapabilities(ctx, &scriptspb.GetAccountCapabilitiesRequest{
		ExecutingAccountHandle: sourceResolveResp.AccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		ScriptName:             scriptName,
	})
	if err != nil {
		return err
	}

	prettyAccountCaps := make([]string, 0)
	if getAccountCapsResp.Capabilities.BillUsageToExecutingAccount {
		prettyAccountCaps = append(prettyAccountCaps, " - "+explainBillUsageToExecutingAccount(c)+" (you!)")
	}

	if getAccountCapsResp.Capabilities.WithdrawalLimit > 0 {
		prettyAccountCaps = append(prettyAccountCaps, " - "+explainWithdrawalLimit(c, channel, getAccountCapsResp.Capabilities.WithdrawalLimit))
	}

	var prettyAccountCapDetails string
	if len(prettyAccountCaps) > 0 {
		prettyAccountCapDetails = fmt.Sprintf("**You have granted your following capabilities:**\n\n%s", strings.Join(prettyAccountCaps, "\n"))
	} else {
		prettyAccountCapDetails = fmt.Sprintf("**You have granted none of your capabilities.**")
	}

	var preamble string
	if aliased {
		preamble = fmt.Sprintf("the command `%s` is an alias for `%s/%s`", commandName, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName)
	} else {
		preamble = fmt.Sprintf("the command `%s/%s`", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>, %s.\n\n%s\n\n%s", m.Author.ID, preamble, prettyRequestedCapDetails, prettyAccountCapDetails))
	return nil
}

func bankSetcaps(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	sourceResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 && len(parts) != 1 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please use `%ssetcaps command capabilities` to set command capabilities.", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	commandName := parts[0]
	capabilities := &scriptspb.Capabilities{}
	if len(parts) == 2 {
		if err := proto.UnmarshalText(parts[1], capabilities); err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand the capabilities you wanted to set. Please use `%ssetcaps command capabilities` to set command capabilities.", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
			return nil
		}
	}

	scriptAccountHandle, scriptName, _, err := resolveScriptName(ctx, c, commandName)
	if err != nil {
		if err == errNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I don't know what the `%s` command is.", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	if _, err := c.scriptsClient.SetAccountCapabilities(ctx, &scriptspb.SetAccountCapabilitiesRequest{
		ExecutingAccountHandle: sourceResolveResp.AccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		ScriptName:             scriptName,
		Capabilities:           capabilities,
	}); err != nil {
		return err
	}

	prettyAccountCaps := make([]string, 0)
	if capabilities.BillUsageToExecutingAccount {
		prettyAccountCaps = append(prettyAccountCaps, " - "+explainBillUsageToExecutingAccount(c)+" (you!)")
	}

	if capabilities.WithdrawalLimit > 0 {
		prettyAccountCaps = append(prettyAccountCaps, " - "+explainWithdrawalLimit(c, channel, capabilities.WithdrawalLimit))
	}

	if len(prettyAccountCaps) > 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>, you have granted your following capabilities to `%s`:\n\n%s", m.Author.ID, commandName, strings.Join(prettyAccountCaps, "\n")))
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>, you have revoked all of your capabilities from `%s`.", m.Author.ID, commandName))
	}

	return nil
}

func bankKey(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I only respond to this command in private.", m.Author.ID))
		return nil
	}

	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, you don't have an account.", m.Author.ID))
			return nil
		}
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>, your account handle (**username**) is `%s` and your account key (**password**) is `%s`. Keep it secret!", m.Author.ID, base64.RawURLEncoding.EncodeToString(resolveResp.AccountHandle), base64.RawURLEncoding.EncodeToString(resolveResp.AccountKey)))
	return nil
}

func bankHelp(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(`Hi <@!%s>, I understand the following commands:

`+"`"+`$balance [@mention/handle]`+"`"+`
_Also available as:_ `+"`"+`$`+"`"+`, `+"`"+`$bal`+"`"+`
Get a user's balance. Leave out the username to get your own balance.

`+"`"+`$account [@mention/handle]`+"`"+`
_Also available as:_ `+"`"+`$$`+"`"+`
Get a user's account information. Leave out the username to get your own accounts.

`+"`"+`$pay @mention/handle amount`+"`"+`
Pay a user from your account into their account.

`+"`"+`$cmd command`+"`"+`
Get information on a command.

`+"`"+`$setcaps command [capabilities]`+"`"+`
Set your capabilities on a command. If capabilities are left empty, all capabilities you have previously granted are revoked.

**I will only respond to the following commands in private:**

`+"`"+`$key`+"`"+`
Gets the key to your account.

**You can also visit me online at %s** For login details, please message me `+"`"+`$key`+"`"+` in private.

`, m.Author.ID, c.opts.WebURL))
	return nil
}
