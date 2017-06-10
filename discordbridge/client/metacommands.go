package client

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	"github.com/bwmarrin/discordgo"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type metaCommand func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error

var metaCommands map[string]metaCommand = map[string]metaCommand{
	"command": metaCmd,
	"cmd":     metaCmd,

	"admin.setalias": metaAdminSetAlias,
	"admin.delalias": metaAdminDelAlias,

	"?":    metaHelp,
	"help": metaHelp,
}

var discordMentionRegexp = regexp.MustCompile(`<@!?(\d+)>`)

func metaCmd(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	commandName := rest

	ownerName, scriptName, aliased, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
	if err != nil {
		if err == errNotFound {
			c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s` not found**", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: ownerName,
		Name:      scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command alias references invalid script name**", m.Author.ID))
			} else {
				c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s/%s` not found**", m.Author.ID, ownerName, scriptName))
			}
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid script name**", m.Author.ID))
			return nil
		}
		return err
	}

	description := getMeta.Meta.Description
	if description == "" {
		description = "_No description available._"
	}

	c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Description: description,
			URL:         fmt.Sprintf("%s/scripts/%s/%s", c.opts.WebURL, ownerName, scriptName),
			Color:       0x009100,
			Author: &discordgo.MessageEmbedAuthor{
				Name: ownerName,
				URL:  fmt.Sprintf("%s/scripts/%s", c.opts.WebURL, ownerName),
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Script Name",
					Value: fmt.Sprintf("`%s/%s`", ownerName, scriptName),
				},
			},
		},
	})

	return nil
}

func memberIsAdmin(adminRoleID string, member *discordgo.Member) bool {
	for _, roleID := range member.Roles {
		if roleID == adminRoleID {
			return true
		}
	}
	return false
}

func metaAdminSetAlias(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	member, err := c.session.GuildMember(channel.GuildID, m.Author.ID)
	if err != nil {
		return err
	}

	if !memberIsAdmin(guildVars.AdminRoleID, member) {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
		return nil
	}

	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `%sadmin.setalias <command name> <qualified script name>`**", m.Author.ID, guildVars.MetaCommandPrefix))
		return nil
	}

	commandName := parts[0]
	if strings.ContainsAny(commandName, "/") {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Alias name must not contain forward slashes**", m.Author.ID))
		return nil
	}

	qualifiedScriptName := parts[1]
	firstSlash := strings.Index(qualifiedScriptName, "/")
	if firstSlash == -1 {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Script name must be of format `<owner name>/<script name>`**", m.Author.ID))
		return nil
	}

	ownerName := qualifiedScriptName[:firstSlash]
	scriptName := qualifiedScriptName[firstSlash+1:]

	getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: ownerName,
		Name:      scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Script not found**", m.Author.ID))
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Invalid script name**", m.Author.ID))
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
		OwnerName:  ownerName,
		ScriptName: scriptName,
	}); err != nil {
		return err
	}

	tx.Commit()

	description := getMeta.Meta.Description
	if description == "" {
		description = "_No description available._"
	}

	c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Description: description,
			URL:         fmt.Sprintf("%s/scripts/%s/%s", c.opts.WebURL, ownerName, scriptName),
			Color:       0x009100,
			Author: &discordgo.MessageEmbedAuthor{
				Name: ownerName,
				URL:  fmt.Sprintf("%s/scripts/%s", c.opts.WebURL, ownerName),
			},
			Fields: []*discordgo.MessageEmbedField{
				{
					Name:  "Script Name",
					Value: fmt.Sprintf("`%s/%s`", ownerName, scriptName),
				},
			},
		},
	})

	return nil
}

func metaAdminDelAlias(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	member, err := c.session.GuildMember(channel.GuildID, m.Author.ID)
	if err != nil {
		return err
	}

	if !memberIsAdmin(guildVars.AdminRoleID, member) {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
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

	c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("`%s`", commandName),
			Color:       0x009100,
			Description: "Deleted.",
		},
	})

	return nil
}

func metaHelp(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
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

	aliasNames := make([]string, 0, len(aliases))
	for aliasName, _ := range aliases {
		aliasNames = append(aliasNames, aliasName)
	}
	sort.Strings(aliasNames)

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	fields := make([]*discordgo.MessageEmbedField, len(aliases))

	var wg sync.WaitGroup

	for i, aliasName := range aliasNames {
		wg.Add(1)

		go func(i int, aliasName string) {
			defer wg.Done()

			alias := aliases[aliasName]

			getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
				OwnerName: alias.OwnerName,
				Name:      alias.ScriptName,
			})

			prefix := fmt.Sprintf("`%s%s`", guildVars.ScriptCommandPrefix, aliasName)

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
		}(i, aliasName)
		i++
	}

	wg.Wait()

	c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title: "Help",
			Description: `Here's a listing of my commands.

For further information, check out the user documentation at https://kobun4.readthedocs.io/en/latest/users/index.html`,
			Color: 0x009100,
			Fields: append(fields,
				&discordgo.MessageEmbedField{
					Name: fmt.Sprintf("`%scommand <command name>`", guildVars.MetaCommandPrefix),
					Value: `**Also available as:** ` + "`" + `cmd` + "`" + `
Get extended information on any command beginning with ` + "`" + guildVars.ScriptCommandPrefix + "`",
				},
				&discordgo.MessageEmbedField{
					Name: fmt.Sprintf("`%sadmin.setalias <command name> <script name>`", guildVars.MetaCommandPrefix),
					Value: `**Administrators only.**
Alias a command name (short name) to a script name (long name). If the alias already exists, it will be replaced.`,
				},
				&discordgo.MessageEmbedField{
					Name: fmt.Sprintf("`%sadmin.delalias <command name>`", guildVars.MetaCommandPrefix),
					Value: `**Administrators only.**
Remove a command name's alias.`,
				},
			),
		},
	})
	return nil
}
