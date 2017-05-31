package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var errInvalidOutput = errors.New("invalid output")

type command func(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error

type Flavor struct {
	BankCommandPrefix   string `json:"bankCommandPrefix"`
	ScriptCommandPrefix string `json:"scriptCommandPrefix"`
	CurrencyName        string `json:"currencyName"`
	Quiet               bool   `json:"quiet"`
}

type Options struct {
	Flavors map[string]Flavor
	Status  string
	WebURL  string
}

type Client struct {
	session *discordgo.Session

	opts *Options

	bridgeServiceTarget string

	accountsClient accountspb.AccountsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

func New(token string, opts *Options, bridgeServiceTarget string, accountsClient accountspb.AccountsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts: opts,

		bridgeServiceTarget: bridgeServiceTarget,

		accountsClient: accountsClient,
		moneyClient:    moneyClient,
		scriptsClient:  scriptsClient,
	}

	session.AddHandler(client.ready)
	session.AddHandler(client.messageCreate)
	session.AddHandler(client.guildCreate)
	session.AddHandler(client.guildMemberAdd)
	session.AddHandler(client.guildMembersChunk)

	if err := session.Open(); err != nil {
		return nil, err
	}

	return client, nil
}

func aliasName(userID string) string {
	return fmt.Sprintf("discord/%s", userID)
}

func (c *Client) bankCommandPrefix(guildID string) string {
	if flavor, ok := c.opts.Flavors[guildID]; ok {
		return flavor.BankCommandPrefix
	}
	return c.opts.Flavors[""].BankCommandPrefix
}

func (c *Client) scriptCommandPrefix(guildID string) string {
	if flavor, ok := c.opts.Flavors[guildID]; ok {
		return flavor.ScriptCommandPrefix
	}
	return c.opts.Flavors[""].ScriptCommandPrefix
}

func (c *Client) currencyName(guildID string) string {
	if flavor, ok := c.opts.Flavors[guildID]; ok {
		return flavor.CurrencyName
	}
	return c.opts.Flavors[""].CurrencyName
}

func (c *Client) Close() {
	c.session.Close()
}

func (c *Client) Session() *discordgo.Session {
	return c.session
}

func (c *Client) ready(s *discordgo.Session, r *discordgo.Ready) {
	glog.Info("Discord ready.")
	status := c.opts.Status
	if status == "" {
		status = "Hi!"
	}
	s.UpdateStatus(0, fmt.Sprintf("%s | Shard %d/%d", status, s.ShardID+1, s.ShardCount))
}

func (c *Client) guildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	ctx := context.Background()

	glog.Info("Guild received, ensuring accounts.")

	for _, member := range g.Members {
		if member.User.Bot {
			continue
		}

		if err := c.ensureAccount(ctx, member.User.ID); err != nil {
			glog.Errorf("Failed to ensure account: %v", err)
		}
	}

	glog.Info("Accounts ensured.")
}

func (c *Client) guildMemberAdd(s *discordgo.Session, g *discordgo.GuildMemberAdd) {
	ctx := context.Background()

	if g.Member.User.Bot {
		return
	}

	if err := c.ensureAccount(ctx, g.Member.User.ID); err != nil {
		glog.Errorf("Failed to ensure account: %v", err)
	}
}

func (c *Client) guildMembersChunk(s *discordgo.Session, g *discordgo.GuildMembersChunk) {
	ctx := context.Background()

	for _, member := range g.Members {
		if member.User.Bot {
			return
		}

		if err := c.ensureAccount(ctx, member.User.ID); err != nil {
			glog.Errorf("Failed to ensure account: %v", err)
		}
	}
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
		fail = true
	}

	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		glog.Errorf("Failed to ensure account: %v", err)
		fail = true
	}

	if strings.HasPrefix(m.Content, c.bankCommandPrefix(channel.GuildID)) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		rest := m.Content[len(c.bankCommandPrefix(channel.GuildID)):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		cmd, ok := bankCommands[commandName]
		if !ok {
			if !c.opts.Flavors[channel.GuildID].Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s` not found**", m.Author.ID, c.bankCommandPrefix(channel.GuildID), commandName))
			}
			return
		}

		if err := cmd(ctx, c, s, m.Message, channel, rest); err != nil {
			glog.Errorf("Failed to run bank command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(m.Content, c.scriptCommandPrefix(channel.GuildID)) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		rest := m.Content[len(c.scriptCommandPrefix(channel.GuildID)):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		if err := c.runScriptCommand(ctx, s, m.Message, channel, 0, commandName, rest); err != nil {
			glog.Errorf("Failed to run command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if err := c.payForMessage(ctx, m.Message); err != nil {
		glog.Errorf("Failed to pay for message: %v", err)
	}
}

type outputFormatter func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error)

var outputFormatters map[string]outputFormatter = map[string]outputFormatter{
	"text": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{
			Fields: []*discordgo.MessageEmbedField{},
		}

		if syscall.WaitStatus(r.WaitStatus).ExitStatus() != 0 {
			embed.Color = 0xb50000
		} else {
			embed.Color = 0x009100
		}

		stdout := r.Stdout
		if len(stdout) > 1500 {
			stdout = stdout[:1500]
		}

		embed.Description = string(stdout)

		return &discordgo.MessageSend{Embed: embed}, nil
	},

	"discord.embed": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		if err := json.Unmarshal(r.Stdout, &embed); err != nil {
			return nil, errInvalidOutput
		}

		return &discordgo.MessageSend{
			Embed: embed,
		}, nil
	},

	"discord.file": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		nulPosition := bytes.IndexByte(r.Stdout, byte(0))
		if nulPosition == -1 {
			return nil, errInvalidOutput
		}

		return &discordgo.MessageSend{
			File: &discordgo.File{
				Name:   string(r.Stdout[:nulPosition]),
				Reader: bytes.NewBuffer(r.Stdout[nulPosition+1:]),
			},
		}, nil
	},
}

func (c *Client) prettyBillingDetails(commandName string, requirements *scriptspb.Requirements, channel *discordgo.Channel, r *scriptspb.ExecuteResponse) string {
	parts := []string{}

	if !requirements.BillUsageToOwner {
		parts = append(parts, fmt.Sprintf("**Usage cost:** %d %s", r.UsageCost, c.currencyName(channel.GuildID)))
	} else {
		// Leave top line empty.
		parts = append(parts, "")
	}

	charges := make([]string, len(r.Charge))
	for i, withdrawal := range r.Charge {
		charges[i] = fmt.Sprintf("%d %s to `%s`", withdrawal.Amount, c.currencyName(channel.GuildID), base64.RawURLEncoding.EncodeToString(withdrawal.TargetAccountHandle))
	}

	if len(r.Charge) > 0 {
		parts = append(parts, fmt.Sprintf("ℹ **Charges:**\n%s", strings.Join(charges, "\n")))
	}

	return strings.Join(parts, "\n\n")
}

func (c *Client) runScriptCommand(ctx context.Context, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, escrowedFunds int64, commandName string, rest string) error {
	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	scriptAccountHandle, scriptName, aliased, err := resolveScriptName(ctx, c, commandName)
	if err != nil {
		switch err {
		case errNotFound:
			if !c.opts.Flavors[channel.GuildID].Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s` not found**", m.Author.ID, c.scriptCommandPrefix(channel.GuildID), commandName))
			}
			return nil
		}
		return err
	}

	getRequirementsResp, err := c.scriptsClient.GetRequirements(ctx, &scriptspb.GetRequirementsRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Command alias references invalid script name**", m.Author.ID, commandName))
			} else if !c.opts.Flavors[channel.GuildID].Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, c.scriptCommandPrefix(channel.GuildID), base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName))
			}
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Invalid command name**", m.Author.ID))
			return nil
		}
		return err
	}
	requirements := getRequirementsResp.Requirements

	resp, err := c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		ExecutingAccountHandle: resolveResp.AccountHandle,
		ExecutingAccountKey:    resolveResp.AccountKey,
		ScriptAccountHandle:    scriptAccountHandle,
		Name:                   scriptName,
		Rest:                   rest,
		Context: &scriptspb.Context{
			BridgeName: "discord",

			Mention: fmt.Sprintf("<@!%s>", m.Author.ID),
			Source:  m.Author.ID,
			Server:  channel.GuildID,
			Channel: m.ChannelID,

			CurrencyName:        c.currencyName(channel.GuildID),
			ScriptCommandPrefix: c.scriptCommandPrefix(channel.GuildID),
			BankCommandPrefix:   c.bankCommandPrefix(channel.GuildID),
		},
		BridgeServiceTarget: c.bridgeServiceTarget,
		EscrowedFunds:       escrowedFunds,
	})

	if err != nil {
		if grpc.Code(err) == codes.FailedPrecondition {
			if requirements.BillUsageToOwner {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command owner does not have enough funds**", m.Author.ID))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Not enough funds**", m.Author.ID))
			}
			return nil
		}
		return err
	}

	waitStatus := syscall.WaitStatus(resp.WaitStatus)

	if waitStatus.ExitStatus() == 0 || waitStatus.ExitStatus() == 2 {
		outputFormatter, ok := outputFormatters[resp.OutputFormat]
		if !ok {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Output format `%s` unknown!** %s", m.Author.ID, resp.OutputFormat, c.prettyBillingDetails(commandName, requirements, channel, resp)))
			return nil
		}

		messageSend, err := outputFormatter(resp)
		if err != nil {
			if err == errInvalidOutput {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Command output was invalid!** %s", m.Author.ID, c.prettyBillingDetails(commandName, requirements, channel, resp)))
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

		messageSend.Content = fmt.Sprintf("<@!%s>: %s %s", m.Author.ID, sigil, c.prettyBillingDetails(commandName, requirements, channel, resp))

		if _, err := s.ChannelMessageSendComplex(m.ChannelID, messageSend); err != nil {
			return err
		}
		return nil
	} else if waitStatus.Signal() == syscall.SIGKILL {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Took too long!** %s", m.Author.ID, c.prettyBillingDetails(commandName, requirements, channel, resp)))
	} else {
		stderr := resp.Stderr
		if len(stderr) > 1500 {
			stderr = stderr[:1500]
		}

		billingDetails := c.prettyBillingDetails(commandName, requirements, channel, resp)
		var embed discordgo.MessageEmbed
		embed.Color = 0xb50000
		if len(stderr) > 0 {
			embed.Description = fmt.Sprintf("```%s```", string(stderr))
		} else {
			embed.Description = "(stderr was empty)"
		}

		s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
			Content: fmt.Sprintf("<@!%s>: ❗ **Error occurred!** %s", m.Author.ID, billingDetails),
			Embed:   &embed,
		})
	}

	return nil
}

func (c *Client) ensureAccount(ctx context.Context, authorID string) error {
	var err error

	for {
		_, err = c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
			Name: aliasName(authorID),
		})

		if grpc.Code(err) != codes.Unavailable {
			break
		}

		glog.Warningf("Temporary failure to ensure account: %v", err)
		time.Sleep(1 * time.Second)
	}

	if err == nil {
		return nil
	}

	if grpc.Code(err) != codes.NotFound {
		return err
	}

	resp, err := c.accountsClient.Create(ctx, &accountspb.CreateRequest{})
	if err != nil {
		return err
	}

	if _, err := c.accountsClient.SetAlias(ctx, &accountspb.SetAliasRequest{
		Name:          aliasName(authorID),
		AccountHandle: resp.AccountHandle,
	}); err != nil {
		return err
	}

	return nil
}

func (c *Client) payForMessage(ctx context.Context, m *discordgo.Message) error {
	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		return err
	}

	resp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	if _, err := c.moneyClient.Add(ctx, &moneypb.AddRequest{
		AccountHandle: resp.AccountHandle,
		Amount:        c.messageEarnings(m.Content),
	}); err != nil {
		return err
	}

	return nil
}

func (c *Client) messageEarnings(content string) int64 {
	return 1
}
