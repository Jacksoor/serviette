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
	f         func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, rest string) error
}

var metaCommands map[string]metaCommand = map[string]metaCommand{
	"help": {
		adminOnly: false,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
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

			fields = append(fields,
				&discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("@%s help", c.session.State.User.Username),
					Value: `Displays this help message.`,
				},
				&discordgo.MessageEmbedField{
					Name:  fmt.Sprintf("@%s info <command name>", c.session.State.User.Username),
					Value: `Get information on any linked command beginning with ` + "`" + guildVars.ScriptCommandPrefix + "`",
				},
			)

			if memberIsAdmin(guildVars.AdminRoleID, member) {
				fields = append(fields,
					&discordgo.MessageEmbedField{
						Name: fmt.Sprintf("@%s link <command name> <script name>", c.session.State.User.Username),
						Value: fmt.Sprintf(`**Administrators only.**
Link a command name to a script name. If the link already exists, it will be replaced. A list of linkable script names can be found at %s/scripts`, c.opts.WebURL),
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
					Title:       "ℹ Help",
					URL:         c.opts.WebURL,
					Description: fmt.Sprintf(`Here's a listing of commands that are linked into this server.%s`, formattedAnnouncement),
					Color:       0x009100,
					Fields:      fields,
				},
			})
			return nil
		},
	},
	"info": {
		adminOnly: false,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
			commandName := rest

			linked := commandNameIsLinked(commandName)
			if !linked {
				return &commandError{
					status: errorStatusUser,
					note:   fmt.Sprintf("Link `%s` not found", commandName),
				}
			}

			ownerName, scriptName, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
			if err != nil {
				if err == errNotFound {
					return &commandError{
						status: errorStatusUser,
						note:   fmt.Sprintf("Link `%s` not found", commandName),
					}
				}
				return err
			}

			getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
				OwnerName: ownerName,
				Name:      scriptName,
			})
			if err != nil {
				switch grpc.Code(err) {
				case codes.NotFound, codes.InvalidArgument:
					return &commandError{
						status: errorStatusScript,
						note:   "Link references invalid script name",
					}
				default:
					return err
				}
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
	"link": {
		adminOnly: true,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
			parts := strings.SplitN(rest, " ", 2)

			if len(parts) != 2 {
				return &commandError{
					status: errorStatusUser,
					note:   "Expecting `link <command name> <qualified script name>`",
				}
			}

			commandName := parts[0]
			if strings.ContainsAny(commandName, "/") {
				return &commandError{
					status: errorStatusUser,
					note:   "Link name must not contain forward slashes",
				}
			}

			qualifiedScriptName := parts[1]
			firstSlash := strings.Index(qualifiedScriptName, "/")
			if firstSlash == -1 {
				return &commandError{
					status: errorStatusUser,
					note:   "Script name must be of format `<owner name>/<script name>",
				}
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
	"unlink": {
		adminOnly: true,
		f: func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, rest string) error {
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
