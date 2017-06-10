package client

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	"github.com/bwmarrin/discordgo"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type adminCommand func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error

var adminCommands map[string]adminCommand = map[string]adminCommand{
	"aliasinfo": metaAdminAliasInfo,
	"setalias":  metaAdminSetAlias,
	"delalias":  metaAdminDelAlias,
}

var discordMentionRegexp = regexp.MustCompile(`<@!?(\d+)>`)

func metaAdminAliasInfo(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
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

func metaAdminSetAlias(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
	parts := strings.SplitN(rest, " ", 2)

	if len(parts) != 2 {
		c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `%ssetalias <command name> <qualified script name>`**", m.Author.ID, adminCommandPrefix))
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
