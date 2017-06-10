package client

import (
	"fmt"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Options struct {
	Status string
	WebURL string
}

type Client struct {
	session *discordgo.Session

	opts *Options

	networkInfoServiceTarget string

	vars *varstore.Store

	scriptsClient scriptspb.ScriptsClient
}

func New(token string, opts *Options, networkInfoServiceTarget string, vars *varstore.Store, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts: opts,

		networkInfoServiceTarget: networkInfoServiceTarget,

		vars: vars,

		scriptsClient: scriptsClient,
	}

	session.AddHandler(client.ready)
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
}

var GuildVars *varstore.GuildVars = &varstore.GuildVars{
	ScriptCommandPrefix: ".",
	MetaCommandPrefix:   "$",
	Quiet:               true,
}

func (c *Client) serverVarsOrDefault(ctx context.Context, guildID string) (*varstore.GuildVars, error) {
	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	guildVars, err := c.vars.GuildVars(ctx, tx, guildID)
	if err != nil {
		if err == varstore.ErrNotFound {
			return GuildVars, nil
		}
		return nil, err
	}

	return guildVars, nil
}

func (c *Client) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()

	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Author.Bot {
		return
	}

	fail := false

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		glog.Errorf("Failed to get channel: %v", err)
		return
	}

	guildVars, err := c.serverVarsOrDefault(ctx, channel.GuildID)
	if err != nil {
		glog.Errorf("Failed to get server vars: %v", err)
		return
	}

	content := strings.TrimSpace(m.Content)

	if content == fmt.Sprintf("<@!%s> help", s.State.User.ID) || content == fmt.Sprintf("<@%s> help", s.State.User.ID) {
		if err := metaHelp(ctx, c, guildVars, m.Message, channel, ""); err != nil {
			glog.Errorf("Failed to run run help command: %v", err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
		}
		return
	}

	if strings.HasPrefix(content, guildVars.MetaCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		rest := content[len(guildVars.MetaCommandPrefix):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		cmd, ok := metaCommands[commandName]
		if !ok {
			if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s` not found**", m.Author.ID, guildVars.MetaCommandPrefix, commandName))
			}
			return
		}

		if err := cmd(ctx, c, guildVars, m.Message, channel, rest); err != nil {
			glog.Errorf("Failed to run meta command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(content, guildVars.ScriptCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		rest := m.Content[len(guildVars.ScriptCommandPrefix):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		if err := c.runScriptCommand(ctx, guildVars, s, m.Message, channel, commandName, rest); err != nil {
			glog.Errorf("Failed to run command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}
}

func (c *Client) runScriptCommand(ctx context.Context, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, commandName string, rest string) error {
	ownerName, scriptName, aliased, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
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

	waitMsg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ⌛ **Please wait, running your command...**", m.Author.ID))
	if err != nil {
		return err
	}
	defer s.ChannelMessageDelete(m.ChannelID, waitMsg.ID)

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
			MetaCommandPrefix:   guildVars.MetaCommandPrefix,
		},
		NetworkInfoServiceTarget: c.networkInfoServiceTarget,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command alias references invalid script name**", m.Author.ID, commandName))
			} else if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, ownerName, scriptName))
			}
			return nil
		}
		return err
	}

	waitStatus := syscall.WaitStatus(resp.WaitStatus)

	if waitStatus.ExitStatus() == 0 || waitStatus.ExitStatus() == 2 {
		outputFormatter, ok := outputFormatters[resp.OutputFormat]
		if !ok {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Output format `%s` unknown!**", m.Author.ID, resp.OutputFormat))
			return nil
		}

		messageSend, err := outputFormatter(resp)
		if err != nil {
			if iErr, ok := err.(invalidOutputError); ok {
				s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
					Content: fmt.Sprintf("<@%s>: ❗ **Command output was invalid!**", m.Author.ID),
					Embed: &discordgo.MessageEmbed{
						Color:       0xb50000,
						Description: fmt.Sprintf("```%s```", iErr.Error()),
					},
				})
				return nil
			}
			return err
		}

		var sigil string
		if syscall.WaitStatus(resp.WaitStatus).ExitStatus() != 0 {
			sigil = "❎"
		} else {
			sigil = "✅"
		}

		messageSend.Content = fmt.Sprintf("<@%s>: %s", m.Author.ID, sigil)

		if _, err := s.ChannelMessageSendComplex(m.ChannelID, messageSend); err != nil {
			return err
		}
		return nil
	} else if waitStatus.Signaled() {
		sig := waitStatus.Signal()
		switch sig {
		case syscall.SIGKILL:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Script used too many resources!**", m.Author.ID))
		default:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Script was killed by %s!**", m.Author.ID, sig.String()))
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

		s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s>: ❗ **Error occurred!**", m.Author.ID),
			Embed:   &embed,
		})
	}

	return nil
}
