package client

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

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

	"command": bankCmd,
	"cmd":     bankCmd,

	"commands": bankCmds,
	"cmds":     bankCmds,

	"key": bankKey,

	"neworphan": bankNeworphan,

	"transfer": bankTransfer,

	"admin.setalias": bankAdminSetAlias,
	"admin.delalias": bankAdminDelAlias,

	"?":    bankHelp,
	"help": bankHelp,
}

var discordMentionRegexp = regexp.MustCompile(`<@!?(\d+)>`)

func bankBalance(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	target := rest

	checkSelf := false
	if target == "" {
		target = fmt.Sprintf("<@!%s>", m.Author.ID)
		checkSelf = true
	}

	var accountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		accountHandle, err = resolveAccountTarget(ctx, tx, c, target)
		if err != nil {
			switch err {
			case errNotFound:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Target user does not have account**", m.Author.ID))
				return false, nil
			case errBadAccountHandle:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
				return false, nil
			}
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Account `%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(accountHandle)))
			return nil
		}
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	prefix := "Your"
	if !checkSelf {
		prefix = fmt.Sprintf("%s's", target)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ✅ **%s balance:** %d %s", m.Author.ID, prefix, resp.Balance, guildVars.CurrencyName))
	return nil
}

func bankAccount(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	target := rest

	checkSelf := false
	if target == "" {
		target = fmt.Sprintf("<@!%s>", m.Author.ID)
		checkSelf = true
	}

	var accountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		accountHandle, err = resolveAccountTarget(ctx, tx, c, target)
		if err != nil {
			switch err {
			case errNotFound:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Target user does not have account**", m.Author.ID))
				return false, nil
			case errBadAccountHandle:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
				return false, nil
			}
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	resp, err := c.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Account `%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(accountHandle)))
			return nil
		}
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	}

	prefix := "Your"
	if !checkSelf {
		prefix = fmt.Sprintf("%s's", target)
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ✅ **%s (`%s`) balance:** %d %s", m.Author.ID, prefix, base64.RawURLEncoding.EncodeToString(accountHandle), resp.Balance, guildVars.CurrencyName))
	return nil
}

func bankPay(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `%spay amount @mention/handle`**", m.Author.ID, guildVars.BankCommandPrefix))
		return nil
	}

	amount, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting numeric amount**", m.Author.ID))
		return nil
	}

	target := parts[1]

	var sourceAccountHandle []byte
	var targetAccountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		userVars, err := c.vars.UserVars(ctx, tx, m.Author.ID)
		if err != nil {
			return false, err
		}

		sourceAccountHandle = userVars.AccountHandle

		targetAccountHandle, err = resolveAccountTarget(ctx, tx, c, target)
		if err != nil {
			switch err {
			case errNotFound:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Target user does not have account**", m.Author.ID))
				return false, nil
			case errBadAccountHandle:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
				return false, nil
			}
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	if _, err := c.moneyClient.Transfer(ctx, &moneypb.TransferRequest{
		SourceAccountHandle: sourceAccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              amount,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Account not found**", m.Author.ID))
			return nil
		case codes.FailedPrecondition:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Not enough funds**", m.Author.ID))
			return nil
		case codes.InvalidArgument:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid transfer amount**", m.Author.ID))
			return nil
		}
		return err
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	} else {
		target = fmt.Sprintf("%s (`%s`)", target, base64.RawURLEncoding.EncodeToString(targetAccountHandle))
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "Payment Receipt",
			Color: 0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "From",
					Value:  fmt.Sprintf("<@!%s> (`%s`)", m.Author.ID, base64.RawURLEncoding.EncodeToString(sourceAccountHandle)),
					Inline: true,
				},
				{
					Name:   "To",
					Value:  target,
					Inline: true,
				},
				{
					Name:  "Amount",
					Value: fmt.Sprintf("%d %s", amount, guildVars.CurrencyName),
				},
			},
		},
	})
	return nil
}

func bankCmd(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	commandName := rest

	scriptAccountHandle, scriptName, aliased, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
	if err != nil {
		if err == errNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s` not found**", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command alias references invalid script name**", m.Author.ID))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s/%s` not found**", m.Author.ID, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName))
			}
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid script name**", m.Author.ID))
			return nil
		}
		return err
	}

	description := getMeta.Meta.Description
	if description == "" {
		description = "_No description available._"
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Description: description,
			URL:         fmt.Sprintf("%s/scripts/%s/%s", c.opts.WebURL, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName),
			Color:       0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Script Name",
					Value: fmt.Sprintf("`%s/%s`", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName),
				},
				{
					Name:   "Usage Billing",
					Value:  explainBillUsageToOwner(c, getMeta.Meta.BillUsageToOwner),
					Inline: true,
				},
				{
					Name:   "Escrow",
					Value:  explainNeedsEscrow(c, getMeta.Meta.NeedsEscrow),
					Inline: true,
				},
			},
		},
	})

	return nil
}

func bankCmds(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	var aliases map[string]*varstore.Alias
	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		aliases, err = c.vars.GuildAliases(ctx, tx, channel.GuildID)
		if err != nil {
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	fields := make([]*discordgo.MessageEmbedField, len(aliases))

	var wg sync.WaitGroup

	i := 0
	for name, alias := range aliases {
		wg.Add(1)

		go func(i int, name string, alias *varstore.Alias) {
			defer wg.Done()

			getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
				AccountHandle: alias.AccountHandle,
				Name:          alias.ScriptName,
			})

			prefix := fmt.Sprintf("`%s%s`", guildVars.ScriptCommandPrefix, name)

			if err != nil {
				if grpc.Code(err) == codes.NotFound {
					fields[i] = &discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("%s (NOT FOUND)", prefix),
						Value: fmt.Sprintf("_Script was not found._"),
					}
					return
				}

				fields[i] = &discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("%s (ERROR)", prefix),
					Value: fmt.Sprintf("_Internal server error._"),
				}
				glog.Errorf("Failed to get script meta: %v", err)
				return
			}

			description := getMeta.Meta.Description
			if description == "" {
				description = "_No description available._"
			}

			fields[i] = &discordgo.MessageEmbedField{
				Name:  prefix,
				Value: description,
			}
		}(i, name, alias)
		i++
	}

	wg.Wait()

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       "Commands",
			Color:       0x009100,
			Description: fmt.Sprintf("These are the available commands. For detailed information, you can use `%scommand <command name>`.", guildVars.BankCommandPrefix),
			Fields:      fields,
		},
	})
	return nil
}

func bankKey(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	var accountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		userVars, err := c.vars.UserVars(ctx, tx, m.Author.ID)
		if err != nil {
			return false, err
		}

		accountHandle = userVars.AccountHandle
		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	getResp, err := c.accountsClient.Get(ctx, &accountspb.GetRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		return err
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "Your Secret Account Details",
			Color: 0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Handle (Username)",
					Value: fmt.Sprintf("`%s`", base64.RawURLEncoding.EncodeToString(accountHandle)),
				},
				{
					Name:  "Key (Password)",
					Value: fmt.Sprintf("`%s`", base64.RawURLEncoding.EncodeToString(getResp.AccountKey)),
				},
			},
		},
	})
	return nil
}

func bankNeworphan(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	resp, err := c.accountsClient.Create(ctx, &accountspb.CreateRequest{})
	if err != nil {
		return err
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "New Orphan Account Details",
			Color: 0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Handle (Username)",
					Value: fmt.Sprintf("`%s`", base64.RawURLEncoding.EncodeToString(resp.AccountHandle)),
				},
				{
					Name:  "Key (Password)",
					Value: fmt.Sprintf("`%s`", base64.RawURLEncoding.EncodeToString(resp.AccountKey)),
				},
			},
		},
	})
	return nil
}

func bankTransfer(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	if !channel.IsPrivate {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Can only use this command in private**", m.Author.ID))
		return nil
	}

	parts := strings.SplitN(rest, " ", 4)

	if len(parts) != 4 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `%stransfer amount source@mention/sourcehandle sourcekey target@mention/targethandle`**", m.Author.ID, guildVars.BankCommandPrefix))
		return nil
	}

	source := parts[1]
	target := parts[3]

	amount, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting numeric amount**", m.Author.ID))
		return nil
	}

	sourceAccountKey, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting account key**", m.Author.ID))
		return nil
	}

	var sourceAccountHandle []byte
	var targetAccountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		sourceAccountHandle, err = resolveAccountTarget(ctx, tx, c, source)
		if err != nil {
			switch err {
			case errNotFound:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Source user does not have account**", m.Author.ID))
				return false, nil
			case errBadAccountHandle:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
				return false, nil
			}
			return false, err
		}

		targetAccountHandle, err = resolveAccountTarget(ctx, tx, c, target)
		if err != nil {
			switch err {
			case errNotFound:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Target user does not have account**", m.Author.ID))
				return false, nil
			case errBadAccountHandle:
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting @mention or account handle**", m.Author.ID))
				return false, nil
			}
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	getResp, err := c.accountsClient.Get(ctx, &accountspb.GetRequest{
		AccountHandle: sourceAccountHandle,
	})

	if err != nil {
		return err
	}

	if string(getResp.AccountKey) != string(sourceAccountKey) {
		if grpc.Code(err) == codes.PermissionDenied {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Account not found**", m.Author.ID))
			return nil
		case codes.FailedPrecondition:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Not enough funds**", m.Author.ID))
			return nil
		case codes.InvalidArgument:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid transfer amount**", m.Author.ID))
			return nil
		}
		return err
	}

	if source[0] != '<' {
		source = "`" + source + "`"
	} else {
		source = fmt.Sprintf("%s (`%s`)", source, base64.RawURLEncoding.EncodeToString(sourceAccountHandle))
	}

	if target[0] != '<' {
		target = "`" + target + "`"
	} else {
		target = fmt.Sprintf("%s (`%s`)", target, base64.RawURLEncoding.EncodeToString(targetAccountHandle))
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "Transfer Receipt",
			Color: 0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:   "From",
					Value:  source,
					Inline: true,
				},
				{
					Name:   "To",
					Value:  target,
					Inline: true,
				},
				{
					Name:  "Amount",
					Value: fmt.Sprintf("%d %s", amount, guildVars.CurrencyName),
				},
			},
		},
	})
	return nil
}

func memberIsAdmin(s *discordgo.Session, adminRoleID string, member *discordgo.Member) bool {
	for _, roleID := range member.Roles {
		if roleID == adminRoleID {
			return true
		}
	}
	return false
}

func bankAdminSetAlias(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	member, err := s.GuildMember(channel.GuildID, m.Author.ID)
	if err != nil {
		return err
	}

	if !memberIsAdmin(s, guildVars.AdminRoleID, member) {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
		return nil
	}

	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `%sadmin.setalias commandname scriptname`**", m.Author.ID, guildVars.BankCommandPrefix))
		return nil
	}

	commandName := parts[0]
	if strings.ContainsAny(commandName, "/") {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Alias name must not contain forward slashes**", m.Author.ID))
		return nil
	}

	qualifiedScriptName := parts[1]
	firstSlash := strings.Index(qualifiedScriptName, "/")
	if firstSlash == -1 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Script name must be of format `<account handle>/<script name>`**", m.Author.ID))
		return nil
	}

	scriptAccountHandle, err := base64.RawURLEncoding.DecodeString(qualifiedScriptName[:firstSlash])
	if err != nil {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Script name must be of format `<account handle>/<script name>`**", m.Author.ID))
		return nil
	}

	scriptName := qualifiedScriptName[firstSlash+1:]

	getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Script not found**", m.Author.ID))
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid script name**", m.Author.ID))
			return nil
		}
		return err
	}

	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := c.vars.SetGuildAlias(ctx, tx, channel.GuildID, commandName, &varstore.Alias{
		AccountHandle: scriptAccountHandle,
		ScriptName:    scriptName,
	}); err != nil {
		return err
	}

	tx.Commit()

	description := getMeta.Meta.Description
	if description == "" {
		description = "_No description available._"
	}

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Description: description,
			URL:         fmt.Sprintf("%s/scripts/%s/%s", c.opts.WebURL, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName),
			Color:       0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Script Name",
					Value: fmt.Sprintf("`%s/%s`", base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName),
				},
				{
					Name:   "Usage Billing",
					Value:  explainBillUsageToOwner(c, getMeta.Meta.BillUsageToOwner),
					Inline: true,
				},
				{
					Name:   "Escrow",
					Value:  explainNeedsEscrow(c, getMeta.Meta.NeedsEscrow),
					Inline: true,
				},
			},
		},
	})

	return nil
}

func bankAdminDelAlias(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	member, err := s.GuildMember(channel.GuildID, m.Author.ID)
	if err != nil {
		return err
	}

	if !memberIsAdmin(s, guildVars.AdminRoleID, member) {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
		return nil
	}

	commandName := rest

	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := c.vars.SetGuildAlias(ctx, tx, channel.GuildID, commandName, nil); err != nil {
		return err
	}

	tx.Commit()

	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Color:       0x009100,
			Description: "Deleted.",
		},
	})

	return nil
}

func bankHelp(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "Help",
			Description: fmt.Sprintf(`Here's a listing of my commands.

You can also visit me online here: %s (for login details, please use the `+"`"+`%skey`+"`"+` command in private)

For further information, check out the user documentation at https://kobun4.readthedocs.io/en/latest/users/index.html`, c.opts.WebURL, guildVars.BankCommandPrefix),
			Color: 0x009100,
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  fmt.Sprintf("`%s<command name> [...]`", guildVars.ScriptCommandPrefix),
					Value: `Runs the command.`,
				},
				{
					Name: fmt.Sprintf("`%scommand <command name>`", guildVars.BankCommandPrefix),
					Value: `**Also available as:** ` + "`" + `cmd` + "`" + `
Get information on the command, which may either be a long name or a short name.`,
				},
				{
					Name: fmt.Sprintf("`%scommands`", guildVars.BankCommandPrefix),
					Value: `**Also available as:** ` + "`" + `cmds` + "`" + `
Get information on all short named commands available.`,
				},
				{
					Name: fmt.Sprintf("`%sbalance [<@mention>/<handle>]`", guildVars.BankCommandPrefix),
					Value: `**Also available as:** ` + "`" + `bal` + "`" + `
Get a user's balance. Leave out the username to get your own balance.`,
				},
				{
					Name: fmt.Sprintf("`%saccount [<@mention>/<handle>]`", guildVars.BankCommandPrefix),
					Value: `**Also available as:** ` + "`" + `$` + "`" + `
Get a user's account information. Leave out the username to get your own accounts.`,
				},
				{
					Name:  fmt.Sprintf("`%spay <amount> [<@mention>/<handle>]`", guildVars.BankCommandPrefix),
					Value: `Send a payment to another user's account.`,
				},
				{
					Name: fmt.Sprintf("`%skey`", guildVars.BankCommandPrefix),
					Value: `**Direct message only.**
Gets the key to your account.`,
				},
				{
					Name: fmt.Sprintf("`%sneworphan`", guildVars.BankCommandPrefix),
					Value: `**Direct message only.**
Creates a new, empty account.`,
				},
				{
					Name: fmt.Sprintf("`%stransfer <amount> <source @mention>/<source handle> <source key> <target @mention>/<target handle>`", guildVars.BankCommandPrefix),
					Value: `**Direct message only.**
Transfer funds directly from the source account into the target account. This requires the key of the source account.`,
				},
				{
					Name: fmt.Sprintf("`%sadmin.setalias <command name> <script name>`", guildVars.BankCommandPrefix),
					Value: `**Administrators only.**
Alias a command name (short name) to a script name (long name). If the alias already exists, it will be replaced.`,
				},
				{
					Name: fmt.Sprintf("`%sadmin.delalias <command name>`", guildVars.BankCommandPrefix),
					Value: `**Administrators only.**
Remove a command name's alias.`,
				},
			},
		},
	})
	return nil
}
