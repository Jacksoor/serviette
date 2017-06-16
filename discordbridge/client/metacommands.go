package client

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	"github.com/bwmarrin/discordgo"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type metaCommand struct {
	adminOnly bool
	f         func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error
}

var metaCommands map[string]metaCommand = map[string]metaCommand{
	"help": metaCommand{
		adminOnly: false,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
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

			// Find all distinct commands to request.
			linkNames := make([]string, 0, len(links))
			for linkName, _ := range links {
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
					formattedNames[j] = guildVars.ScriptCommandPrefix + linkNames[k]
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
					Value: description,
				}
			}

			sort.Sort(ByFieldName(fields))

			member, err := c.session.GuildMember(channel.GuildID, m.Author.ID)
			if err != nil {
				return err
			}

			fields = append(fields,
				&discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("@%s help", c.session.State.User.Username),
					Value: `Displays this help message.`,
				},
				&discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("@%s info <command name>", c.session.State.User.Username),
					Value: `Get link information on any command beginning with ` + "`" + guildVars.ScriptCommandPrefix + "`",
				},
			)

			if memberIsAdmin(guildVars.AdminRoleID, member) {
				fields = append(fields,
					&discordgo.MessageEmbedField{
						Name: fmt.Sprintf("@%s link <command name> <script name>", c.session.State.User.Username),
						Value: `**Administrators only.**
Link a command name to a script name. If the link already exists, it will be replaced.`,
					},
					&discordgo.MessageEmbedField{
						Name: fmt.Sprintf("@%s unlink <command name>", c.session.State.User.Username),
						Value: `**Administrators only.**
Remove a command name link.`,
					},
				)
			}

			formattedAnnouncement := ""
			if guildVars.Announcement != "" {
				formattedAnnouncement = fmt.Sprintf("\n\n📣 **%s**", guildVars.Announcement)
			}

			c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
				Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
				Embed: &discordgo.MessageEmbed{
					Title: "ℹ Help",
					URL:   c.opts.WebURL,
					Description: fmt.Sprintf(`Here's a listing of commands that are linked on this server.

More commands may be available. The full list of commands and their source listings are available at %s/commands%s`, c.opts.WebURL, formattedAnnouncement),
					Color:  0x009100,
					Fields: fields,
				},
			})
			return nil
		},
	},
	"info": metaCommand{
		adminOnly: false,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
			commandName := rest

			linked := commandNameIsLinked(commandName)
			ownerName, scriptName, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
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
					if linked {
						c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command link references invalid script name**", m.Author.ID))
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
		},
	},
	"link": metaCommand{
		adminOnly: true,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
			parts := strings.SplitN(rest, " ", 2)

			if len(parts) != 2 {
				c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Expecting `link <command name> <qualified script name>`**", m.Author.ID))
				return nil
			}

			commandName := parts[0]
			if strings.ContainsAny(commandName, "/") {
				c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Link name must not contain forward slashes**", m.Author.ID))
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

			if err := c.vars.SetGuildLink(ctx, tx, channel.GuildID, commandName, &varstore.Link{
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
		},
	},
	"unlink": metaCommand{
		adminOnly: true,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, rest string) error {
			commandName := rest

			tx, err := c.vars.BeginTx(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			if err := c.vars.SetGuildLink(ctx, tx, channel.GuildID, commandName, nil); err != nil {
				return err
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
		},
	},
}
