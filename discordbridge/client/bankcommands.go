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

	"grant": bankGrant,

	"key": bankKey,

	"neworphan": bankNeworphan,

	"transfer": bankTransfer,

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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Target user does not have account**", m.Author.ID))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
			return nil
		}
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Account `%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(accountHandle)))
			return nil
		}
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **%s's balance:** %d %s", m.Author.ID, target, resp.Balance, c.currencyName(channel.GuildID)))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Target user does not have account**", m.Author.ID))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
			return nil
		}
		return err
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Account `%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(accountHandle)))
			return nil
		}
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **%s (`%s`)'s balance:** %d %s", m.Author.ID, target, base64.RawURLEncoding.EncodeToString(accountHandle), resp.Balance, c.currencyName(channel.GuildID)))
	return nil
}

func bankPay(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting `%spay @mention/handle amount`**", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	amount, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting numeric amount**", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Target user does not have account**", m.Author.ID))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
			return nil
		}
		return err
	}

	if _, err := c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceResolveResp.AccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              amount,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Account not found**", m.Author.ID))
			return nil
		case codes.FailedPrecondition:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Not enough funds**", m.Author.ID))
			return nil
		case codes.InvalidArgument:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Invalid transfer amount**", m.Author.ID))
			return nil
		}
		return err
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Payment sent to %s:** %d %s", m.Author.ID, target, amount, c.currencyName(channel.GuildID)))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s` not found**", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	getRequirements, err := c.scriptsClient.GetRequirements(ctx, &scriptspb.GetRequirementsRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Command alias references invalid script name**", m.Author.ID, commandName))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s/%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName))
			}
			return nil
		}
		return err
	}

	prettyRequirements := make([]string, 0)
	if getRequirements.Requirements.BillUsageToExecutingAccount {
		prettyRequirements = append(prettyRequirements, " - "+explainBillUsageToExecutingAccount(c))
	}

	var prettyRequirementDetails string
	if len(prettyRequirements) > 0 {
		prettyRequirementDetails = fmt.Sprintf("**Requirements:**\n%s", strings.Join(prettyRequirements, "\n"))
	} else {
		prettyRequirementDetails = fmt.Sprintf("**No requirements imposed.**")
	}

	getGrantsResp, err := c.scriptsClient.GetGrants(ctx, &scriptspb.GetGrantsRequest{
		ExecutingAccountHandle: sourceResolveResp.AccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		ScriptName:             scriptName,
	})
	if err != nil {
		return err
	}

	prettyGrants := make([]string, 0)
	if getGrantsResp.Grants.BillUsageToExecutingAccount {
		prettyGrants = append(prettyGrants, " - "+explainBillUsageToExecutingAccount(c)+" (you!)")
	}

	if getGrantsResp.Grants.WithdrawalLimit > 0 {
		prettyGrants = append(prettyGrants, fmt.Sprintf(" - maximum withdrawal limit of %d %s", getGrantsResp.Grants.WithdrawalLimit, c.currencyName(channel.GuildID)))
	}

	var prettyGrantsDetails string
	if len(prettyGrants) > 0 {
		prettyGrantsDetails = fmt.Sprintf("**Grants:**\n%s", strings.Join(prettyGrants, "\n"))
	} else {
		prettyGrantsDetails = fmt.Sprintf("**No grants bestowed.**")
	}

	var preamble string
	if aliased {
		preamble = fmt.Sprintf("`%s` (`%s/%s`)", commandName, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName)
	} else {
		preamble = fmt.Sprintf("`%s/%s`", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Command information for %s:**\n\n%s\n\n%s", m.Author.ID, preamble, prettyRequirementDetails, prettyGrantsDetails))
	return nil
}

func bankGrant(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	sourceResolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 && len(parts) != 1 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting `%sgrant command grants`**", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	commandName := parts[0]
	grants := &scriptspb.Grants{}
	if len(parts) == 2 {
		if err := proto.UnmarshalText(parts[1], grants); err != nil {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Invalid grants specified**", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
			return nil
		}
	}

	scriptAccountHandle, scriptName, _, err := resolveScriptName(ctx, c, commandName)
	if err != nil {
		if err == errNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s` not found**", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	if _, err := c.scriptsClient.SetGrants(ctx, &scriptspb.SetGrantsRequest{
		ExecutingAccountHandle: sourceResolveResp.AccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		ScriptName:             scriptName,
		Grants:                 grants,
	}); err != nil {
		return err
	}

	prettyGrants := make([]string, 0)
	if grants.BillUsageToExecutingAccount {
		prettyGrants = append(prettyGrants, " - "+explainBillUsageToExecutingAccount(c)+" (you!)")
	}

	if grants.WithdrawalLimit > 0 {
		prettyGrants = append(prettyGrants, fmt.Sprintf(" - maximum withdrawal limit of %d %s", grants.WithdrawalLimit, c.currencyName(channel.GuildID)))
	}

	if len(prettyGrants) > 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Grants bestowed on `%s`:**\n%s", m.Author.ID, commandName, strings.Join(prettyGrants, "\n")))
	} else {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **All grants revoked from `%s`**", m.Author.ID, commandName))
	}

	return nil
}

func bankKey(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Account handle (username):** `%s`,  **Account key (password):** `%s`", m.Author.ID, base64.RawURLEncoding.EncodeToString(resolveResp.AccountHandle), base64.RawURLEncoding.EncodeToString(resolveResp.AccountKey)))
	return nil
}

func bankNeworphan(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	resp, err := c.accountsClient.Create(ctx, &accountspb.CreateRequest{})
	if err != nil {
		return err
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Account handle (username):** `%s`,  **Account key (password):** `%s`", m.Author.ID, base64.RawURLEncoding.EncodeToString(resp.AccountHandle), base64.RawURLEncoding.EncodeToString(resp.AccountKey)))
	return nil
}

func bankTransfer(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	parts := strings.SplitN(rest, " ", 4)

	if len(parts) != 4 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting `%stransfer source@mention/sourcehandle sourcekey target@mention/targethandle amount`**", m.Author.ID, c.bankCommandPrefix(channel.GuildID)))
		return nil
	}

	amount, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting numeric amount**", m.Author.ID))
		return nil
	}

	sourceAccountKey, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting account key**", m.Author.ID))
		return nil
	}

	source := parts[0]
	sourceAccountHandle, err := resolveAccountTarget(ctx, c, source)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Source user does not have account**", m.Author.ID))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
			return nil
		}
		return err
	}

	target := parts[2]
	targetAccountHandle, err := resolveAccountTarget(ctx, c, target)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Target user does not have account**", m.Author.ID))
			return nil
		case errBadAccountHandle:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
			return nil
		}
		return err
	}

	if _, err := c.accountsClient.Check(context.Background(), &accountspb.CheckRequest{
		AccountHandle: sourceAccountHandle,
		AccountKey:    sourceAccountKey,
	}); err != nil {
		if grpc.Code(err) == codes.PermissionDenied {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Not authorized**", m.Author.ID))
			return nil
		}
		return err
	}

	if _, err := c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceAccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              amount,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Account not found**", m.Author.ID))
			return nil
		case codes.FailedPrecondition:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Not enough funds**", m.Author.ID))
			return nil
		case codes.InvalidArgument:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Invalid transfer amount**", m.Author.ID))
			return nil
		}
		return err
	}

	if source[0] != '<' {
		source = "`" + source + "`"
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ✅ **Transfer from %s to %s:** %d %s", m.Author.ID, source, target, amount, c.currencyName(channel.GuildID)))
	return nil
}

func bankHelp(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(`Hi <@!%s>, I understand the following commands:

`+"`"+`balance [@mention/handle]`+"`"+`
_Also available as:_ `+"`"+`bal`+"`"+`
Get a user's balance. Leave out the username to get your own balance.

`+"`"+`account [@mention/handle]`+"`"+`
_Also available as:_ `+"`"+`$`+"`"+`
Get a user's account information. Leave out the username to get your own accounts.

`+"`"+`pay @mention/handle amount`+"`"+`
Pay a user from your account into their account.

`+"`"+`cmd command`+"`"+`
Get information on a command.

`+"`"+`grant command [grants]`+"`"+`
Set your grants on a command. If grants are left empty, all grants you have previously granted are revoked.

**I will only respond to the following commands in private:**

`+"`"+`key`+"`"+`
Gets the key to your account.

`+"`"+`neworphan`+"`"+`
Creates a new, empty account.

`+"`"+`transfer source@mention/sourcehandle sourcekey target@mention/targethandle amount`+"`"+`
Transfer funds directly from the source account into the target account. This requires the key of the source account.

**You can also visit me online at %s** For login details, please use the `+"`"+`key`+"`"+` command in private.

`, m.Author.ID, c.opts.WebURL))
	return nil
}
