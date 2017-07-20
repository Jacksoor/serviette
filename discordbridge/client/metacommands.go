package client

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	"github.com/bwmarrin/discordgo"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type metaCommand func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error

func adminOnly(f metaCommand) metaCommand {
	return func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		if !c.memberIsAdmin(guildVars, guild, member) {
			return &commandError{
				status: errorStatusUnauthorized,
				note:   "Not authorized",
			}
		}

		return f(ctx, c, guildVars, m, guild, channel, member, rest)
	}
}

var metaCommands map[string]metaCommand = map[string]metaCommand{
	"help": func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		var links map[string]*varstore.Link
		ok, err := func() (bool, error) {
			tx, err := c.vars.BeginTx(ctx)
			if err != nil {
				return false, err
			}
			defer tx.Rollback()

			links, err = c.vars.GuildLinks(ctx, tx, channel.GuildID)
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

		isAdmin := c.memberIsAdmin(guildVars, guild, member)

		prelude := "Here's a listing of commands that are linked into this server."
		if len(links) == 0 {
			prelude = "There aren't any commands linked into this server yet."
			if isAdmin {
				prelude += fmt.Sprintf(" Check out the [script library](%s/scripts) to link some!", c.opts.HomeURL)
			}
		}

		// Find all distinct commands to request.
		linkNames := make([]string, 0, len(links))
		for linkName := range links {
			linkNames = append(linkNames, linkName)
		}

		linkGroups := make(map[string][]int, 0)
		for i, linkName := range linkNames {
			link := links[linkName]

			qualifiedName := fmt.Sprintf("%s/%s", link.OwnerName, link.ScriptName)
			if linkGroups[qualifiedName] == nil {
				linkGroups[qualifiedName] = make([]int, 0)
			}
			linkGroups[qualifiedName] = append(linkGroups[qualifiedName], i)
		}

		uniqueLinks := make([]*varstore.Link, 0, len(linkGroups))
		for _, indexes := range linkGroups {
			uniqueLinks = append(uniqueLinks, links[linkNames[indexes[0]]])
		}

		uniqueLinkMetas := make([]*scriptspb.Meta, len(linkGroups))

		g, ctx := errgroup.WithContext(ctx)

		for i, link := range uniqueLinks {
			i := i
			link := link

			g.Go(func() error {
				getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
					OwnerName: link.OwnerName,
					Name:      link.ScriptName,
				})

				if err != nil {
					if grpc.Code(err) == codes.NotFound {
						return nil
					}
					return err
				}
				if getMeta.Meta.Visibility == scriptspb.Visibility_UNPUBLISHED {
					return nil
				}

				uniqueLinkMetas[i] = getMeta.Meta
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}

		fields := make([]*discordgo.MessageEmbedField, len(uniqueLinks))
		for i, link := range uniqueLinks {
			qualifiedName := fmt.Sprintf("%s/%s", link.OwnerName, link.ScriptName)
			group := linkGroups[qualifiedName]

			formattedNames := make([]string, len(group))
			for j, k := range group {
				formattedNames[j] = linkNames[k]
			}

			meta := uniqueLinkMetas[i]
			description := "**Command not found. Contact an administrator.**"
			if meta != nil {
				description = strings.TrimSpace(meta.Description)
				if description == "" {
					description = "_No description set._"
				}
			}

			sort.Strings(formattedNames)

			fields[i] = &discordgo.MessageEmbedField{
				Name:  strings.Join(formattedNames, ", "),
				Value: fmt.Sprintf("[`[%s]`](%s/scripts/view.html?%s) %s", qualifiedName, c.opts.HomeURL, qualifiedName, description),
			}
		}

		sort.Sort(ByFieldName(fields))

		prefix := fmt.Sprintf("@%s", c.session.State.User.Username)

		if isAdmin {
			fields = append(fields,
				&discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("%s adminhelp", prefix),
					Value: `Help for administrative commands. **You have administrative permissions for Kobun on this server.**`,
				},
			)
		}

		formattedAnnouncement := ""
		if guildVars.Announcement != "" {
			formattedAnnouncement = fmt.Sprintf("\n\n📣 **%s**", guildVars.Announcement)
		}

		numTotalMembers := 0
		for _, guild := range c.session.State.Guilds {
			numTotalMembers += len(guild.Members)
		}

		c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
			Embed: &discordgo.MessageEmbed{
				Title: "ℹ Help",
				URL:   c.opts.HomeURL,
				Description: fmt.Sprintf(`[**Kobun**](%s) is a multipurpose extensible utility bot that [you can program](%s/guides/scripting)!%s

%s`, c.opts.HomeURL, c.opts.HomeURL, formattedAnnouncement, prelude),
				Color:  0x009100,
				Fields: fields,
				Footer: &discordgo.MessageEmbedFooter{
					Text: fmt.Sprintf("Shard %d of %d, running on %d servers with %d members", c.session.ShardID+1, c.session.ShardCount, len(c.session.State.Guilds), numTotalMembers),
				},
			},
		})
		return nil
	},
	"adminhelp": adminOnly(func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		prefix := fmt.Sprintf("@%s", c.session.State.User.Username)

		c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
			Embed: &discordgo.MessageEmbed{
				Title:       "🛂 Administrative Help",
				URL:         c.opts.HomeURL,
				Description: fmt.Sprintf(`**You have administrative permissions for Kobun on this server.** Here is a list of administrative commands you can use.`),
				Color:       0x009100,
				Fields: []*discordgo.MessageEmbedField{
					&discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("%s link <command name> <script name>", prefix),
						Value: fmt.Sprintf(`Link a command name to a script name from the [script library](%s/scripts). If the link already exists, it will be replaced.`, c.opts.HomeURL),
					},
					&discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("%s unlink <command name>", prefix),
						Value: `Remove a command name link.`,
					},
					&discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("%s run <owner name>/<script name> [<input>]", prefix),
						Value: `Run a script. If you are the owner of the script, you may run it even if it is unpublished.`,
					},
				},
			},
		})
		return nil
	}),
	"ping": func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		startTime := time.Now()
		ms, err := c.session.ChannelMessageSend(m.ChannelID, "🏓 **Waiting for pong...**")
		if err != nil {
			return err
		}
		endTime := time.Now()

		c.session.ChannelMessageEdit(m.ChannelID, ms.ID, fmt.Sprintf("🏓 **Pong!** %dms", endTime.Sub(startTime)/time.Millisecond))

		return nil
	},
	"link": adminOnly(func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		parts := strings.SplitN(rest, " ", 2)

		if len(parts) != 2 {
			return &commandError{
				status: errorStatusUser,
				note:   "Expecting `link <command name> <qualified script name>`",
			}
		}

		commandName := parts[0]

		qualifiedScriptName := parts[1]
		firstSlash := strings.Index(qualifiedScriptName, "/")
		if firstSlash == -1 {
			return &commandError{
				status: errorStatusUser,
				note:   "Script name must be of format `<owner name>/<script name>",
			}
		}

		ownerName := qualifiedScriptName[:firstSlash]
		scriptName := qualifiedScriptName[firstSlash+1:]

		getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
			OwnerName: ownerName,
			Name:      scriptName,
		})
		if err != nil {
			if grpc.Code(err) == codes.NotFound {
				return &commandError{
					status: errorStatusUser,
					note:   "Script not found",
				}
			} else if grpc.Code(err) == codes.InvalidArgument {
				return &commandError{
					status: errorStatusUser,
					note:   "Invalid script name",
				}
			}
			return err
		}
		if getMeta.Meta.Visibility == scriptspb.Visibility_UNPUBLISHED {
			return &commandError{
				status: errorStatusScript,
				note:   "Script not found",
			}
		}

		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if err := c.vars.SetGuildLink(ctx, tx, channel.GuildID, commandName, &varstore.Link{
			OwnerName:  ownerName,
			ScriptName: scriptName,
		}); err != nil {
			if err == varstore.ErrInvalid {
				return &commandError{
					status: errorStatusScript,
					note:   "Invalid name for command link",
				}
			}
			return err
		}

		refcount, err := c.vars.Refcount(ctx, tx, channel.GuildID, ownerName, scriptName)
		if err != nil {
			return nil
		}

		if refcount == 1 {
			if _, err := c.scriptsClient.Vote(ctx, &scriptspb.VoteRequest{
				OwnerName: ownerName,
				Name:      scriptName,
				Delta:     1,
			}); err != nil {
				return err
			}
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
				URL:         fmt.Sprintf("%s/scripts/view.html?%s/%s", c.opts.HomeURL, ownerName, scriptName),
				Color:       0x009100,
				Author: &discordgo.MessageEmbedAuthor{
					Name: ownerName,
					URL:  fmt.Sprintf("%s/scripts/%s", c.opts.HomeURL, ownerName),
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
	}),
	"run": adminOnly(func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		parts := strings.SplitN(rest, " ", 2)

		if len(parts) < 1 {
			return &commandError{
				status: errorStatusUser,
				note:   "Expecting `run <command name> [<input>]`",
			}
		}

		var input string
		if len(parts) == 2 {
			input = parts[1]
		}

		scriptParts := strings.SplitN(parts[0], "/", 2)
		if len(scriptParts) != 2 {
			return &commandError{
				status: errorStatusUser,
				note:   "Invalid script name format",
			}
		}

		return c.runScriptCommand(ctx, guildVars, m, guild, channel, member, parts[0], &varstore.Link{OwnerName: scriptParts[0], ScriptName: scriptParts[1]}, input)
	}),
	"unlink": adminOnly(func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
		commandName := rest

		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		link, err := c.vars.GuildLink(ctx, tx, channel.GuildID, commandName)
		if err != nil {
			if err == varstore.ErrNotFound {
				return &commandError{
					status: errorStatusUser,
					note:   "Link not found",
				}
			}
			return err
		}

		if err := c.vars.SetGuildLink(ctx, tx, channel.GuildID, commandName, nil); err != nil {
			return err
		}

		refcount, err := c.vars.Refcount(ctx, tx, channel.GuildID, link.OwnerName, link.ScriptName)
		if err != nil {
			return nil
		}

		if refcount == 0 {
			if _, err := c.scriptsClient.Vote(ctx, &scriptspb.VoteRequest{
				OwnerName: link.OwnerName,
				Name:      link.ScriptName,
				Delta:     -1,
			}); err != nil {
				return err
			}
		}

		tx.Commit()

		c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
			Embed: &discordgo.MessageEmbed{
				Title:       fmt.Sprintf("`%s`", commandName),
				Color:       0x009100,
				Description: "Link removed.",
			},
		})

		return nil
	}),
}
