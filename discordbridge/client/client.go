package client

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/statsstore"
	"github.com/porpoises/kobun4/discordbridge/varstore"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Options struct {
	Status string
	WebURL string
}

type Client struct {
	session *discordgo.Session

	opts            *Options
	knownGuildsOnly bool
	rpcTarget       net.Addr

	vars  *varstore.Store
	stats *statsstore.Store

	scriptsClient scriptspb.ScriptsClient

	metaCommandRegexp *regexp.Regexp
}

func New(token string, opts *Options, knownGuildsOnly bool, rpcTarget net.Addr, vars *varstore.Store, stats *statsstore.Store, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts:            opts,
		knownGuildsOnly: knownGuildsOnly,
		rpcTarget:       rpcTarget,

		vars:  vars,
		stats: stats,

		scriptsClient: scriptsClient,
	}

	session.AddHandler(client.ready)
	session.AddHandler(client.guildCreate)
	session.AddHandler(client.messageCreate)

	if err := session.Open(); err != nil {
		return nil, err
	}

	return client, nil
}

func (c *Client) Close() {
	c.session.Close()
}

func (c *Client) Session() *discordgo.Session {
	return c.session
}

func (c *Client) ready(s *discordgo.Session, r *discordgo.Ready) {
	glog.Info("Discord ready.")
	s.UpdateStatus(0, c.opts.Status)
	c.metaCommandRegexp = regexp.MustCompile(fmt.Sprintf(`^<@!?%s>(.*)$`, regexp.QuoteMeta(s.State.User.ID)))
}

func memberIsAdmin(adminRoleID string, member *discordgo.Member) bool {
	if member == nil {
		return false
	}

	for _, roleID := range member.Roles {
		if roleID == adminRoleID {
			return true
		}
	}
	return false
}

func (c *Client) guildCreate(s *discordgo.Session, m *discordgo.GuildCreate) {
	ctx := context.Background()

	var guildVars *varstore.GuildVars
	if err := func() error {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		guildVars, err = c.vars.GuildVars(ctx, tx, m.Guild.ID)
		if err != nil {
			return err
		}

		return nil
	}(); err != nil {
		if err != varstore.ErrNotFound {
			panic(fmt.Sprintf("Failed to get guild vars: %v", err))
		}
		if c.knownGuildsOnly {
			glog.Warningf("No guild vars found for %s, leaving.", m.Guild.ID)
			s.GuildLeave(m.Guild.ID)
		} else {
			glog.Warningf("No guild vars found for %s, staying anyway.", m.Guild.ID)
		}
	} else {
		glog.Infof("Guild vars for %s: %+v", m.Guild.ID, guildVars)
	}
}

var privateGuildVars = &varstore.GuildVars{
	ScriptCommandPrefix: "",
	Quiet:               false,
}

var unknownGuildVars = &varstore.GuildVars{
	ScriptCommandPrefix: ".",
	Quiet:               true,
}

func (c *Client) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()

	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Author.Bot {
		return
	}

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		glog.Errorf("Failed to get channel: %v", err)
		return
	}

	var guildVars *varstore.GuildVars
	if channel.IsPrivate {
		guildVars = privateGuildVars
	} else {
		if err := func() error {
			tx, err := c.vars.BeginTx(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			guildVars, err = c.vars.GuildVars(ctx, tx, channel.GuildID)
			if err != nil {
				if err == varstore.ErrNotFound {
					guildVars = unknownGuildVars
					return nil
				}
				return err
			}

			return nil
		}(); err != nil {
			glog.Errorf("Failed to get guild vars: %v", err)
			return
		}
	}

	content := strings.TrimSpace(m.Content)

	var member *discordgo.Member
	if channel.GuildID != "" {
		member, err = c.session.GuildMember(channel.GuildID, m.Author.ID)
		if err != nil {
			glog.Errorf("Failed to get member: %v", err)
			return
		}

		if match := c.metaCommandRegexp.FindStringSubmatch(content); match != nil {
			rest := strings.TrimSpace(match[1])
			firstSpaceIndex := strings.Index(rest, " ")

			var commandName string
			if firstSpaceIndex == -1 {
				commandName = rest
				rest = ""
			} else {
				commandName = rest[:firstSpaceIndex]
				rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
			}

			var cmd metaCommand
			var ok bool
			if commandName == "" {
				cmd, ok = metaCommands["help"]
			} else {
				cmd, ok = metaCommands[commandName]
			}

			if !ok {
				if !guildVars.Quiet {
					s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Meta command `%s` not found**", m.Author.ID, commandName))
				}
				return
			}

			if cmd.adminOnly && !memberIsAdmin(guildVars.AdminRoleID, member) {
				c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
				return
			}

			if err := cmd.f(ctx, c, guildVars, m.Message, channel, member, rest); err != nil {
				glog.Errorf("Failed to run command %s: %v", commandName, err)
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
				return
			}

			return
		}
	}

	if strings.HasPrefix(content, guildVars.ScriptCommandPrefix) {
		rest := strings.TrimSpace(m.Content[len(guildVars.ScriptCommandPrefix):])
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		if err := c.runScriptCommand(ctx, guildVars, s, m.Message, channel, member, commandName, rest); err != nil {
			glog.Errorf("Failed to run command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if err := c.stats.RecordUserChannelMessage(ctx, m.Author.ID, channel.ID, int64(len(m.Message.Content))); err != nil {
		glog.Errorf("Failed to record stats: %v", err)
	}
}

type ByFieldName []*discordgo.MessageEmbedField

func (s ByFieldName) Len() int {
	return len(s)
}

func (s ByFieldName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s ByFieldName) Less(i, j int) bool {
	return s[i].Name < s[j].Name
}

func (c *Client) runScriptCommand(ctx context.Context, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, member *discordgo.Member, commandName string, rest string) error {
	linked := commandNameIsLinked(commandName)
	if member != nil && !linked && !memberIsAdmin(guildVars.AdminRoleID, member) {
		if !guildVars.Quiet {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Only the server's Kobun administrators can run unlinked commands**", m.Author.ID))
		}
		return nil
	}

	ownerName, scriptName, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
	if err != nil {
		switch err {
		case errNotFound:
			if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, commandName))
			}
			return nil
		}
		return err
	}

	if _, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: ownerName,
		Name:      scriptName,
	}); err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			if linked {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command link references invalid script name**", m.Author.ID))
			} else if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, ownerName, scriptName))
			}
			return nil
		case codes.Unavailable:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ⚠ **Currently unavailable, please try again later**", m.Author.ID))
			return nil
		default:
			return err
		}
	}

	s.ChannelTyping(m.ChannelID)
	resp, err := c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		OwnerName: ownerName,
		Name:      scriptName,
		Stdin:     []byte(rest),
		Context: &scriptspb.Context{
			BridgeName:  "discord",
			CommandName: commandName,

			UserId:    m.Author.ID,
			ChannelId: m.ChannelID,
			GroupId:   channel.GuildID,
			NetworkId: "discord",

			ScriptCommandPrefix: guildVars.ScriptCommandPrefix,
		},
		BridgeTarget: c.rpcTarget.String(),
	})
	if err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			if linked {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command link references invalid script name**", m.Author.ID))
			} else if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, ownerName, scriptName))
			}
			return nil
		case codes.Unavailable:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ⚠ **Currently unavailable, please try again later**", m.Author.ID))
			return nil
		default:
			return err
		}
	}

	channelID := m.ChannelID
	if resp.OutputParams.Private {
		channel, err := s.UserChannelCreate(m.Author.ID)
		if err != nil {
			return err
		}
		channelID = channel.ID
	}

	waitStatus := syscall.WaitStatus(resp.WaitStatus)

	if waitStatus.ExitStatus() == 0 || waitStatus.ExitStatus() == 2 {
		outputFormatter, ok := OutputFormatters[resp.OutputParams.Format]
		if !ok {
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s> ❗ **Output format `%s` unknown!**", m.Author.ID, resp.OutputParams.Format))
			return nil
		}

		if len(resp.Stdout) == 0 {
			return nil
		}

		messageSend, err := outputFormatter(m.Author.ID, resp.Stdout, syscall.WaitStatus(resp.WaitStatus).ExitStatus() == 0)
		if err != nil {
			if iErr, ok := err.(invalidOutputError); ok {
				s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
					Content: fmt.Sprintf("<@%s> ❗ **Command output was invalid!**", m.Author.ID),
					Embed: &discordgo.MessageEmbed{
						Color:       0xb50000,
						Description: fmt.Sprintf("```%s```", iErr.Error()),
					},
				})
				return nil
			}
			return err
		}

		if _, err := s.ChannelMessageSendComplex(channelID, messageSend); err != nil {
			return err
		}
		return nil
	} else if waitStatus.Signaled() {
		sig := waitStatus.Signal()
		switch sig {
		case syscall.SIGKILL:
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s> ❗ **Script used too many resources!**", m.Author.ID))
		default:
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s> ❗ **Script was killed by %s!**", m.Author.ID, sig.String()))
		}
	} else {
		stderr := resp.Stderr
		if len(stderr) > 1500 {
			stderr = stderr[:1500]
		}

		var embed discordgo.MessageEmbed
		embed.Color = 0xb50000
		if len(stderr) > 0 {
			embed.Description = fmt.Sprintf("```%s```", string(stderr))
		} else {
			embed.Description = "(stderr was empty)"
		}

		s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s> ❗ **Error occurred!**", m.Author.ID),
			Embed:   &embed,
		})
	}

	return nil
}
