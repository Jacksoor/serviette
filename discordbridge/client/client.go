package client

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type command func(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error

type Options struct {
	BankCommandPrefix   string
	ScriptCommandPrefix string
	Status              string
	CurrencyName        string
}

type Client struct {
	opts *Options

	session *discordgo.Session

	accountsClient accountspb.AccountsClient
	deedsClient    deedspb.DeedsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

func New(token string, opts *Options, accountsClient accountspb.AccountsClient, deedsClient deedspb.DeedsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		opts: opts,

		session: session,

		accountsClient: accountsClient,
		deedsClient:    deedsClient,
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

func (c *Client) Close() {
	c.session.Close()
}

func (c *Client) ready(s *discordgo.Session, r *discordgo.Ready) {
	glog.Info("Discord ready.")
	s.UpdateStatus(0, c.opts.Status)
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
	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		glog.Errorf("Failed to ensure account: %v", err)
		fail = true
	}

	if strings.HasPrefix(m.Content, c.opts.BankCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I'm broken right now and can't process your command.", m.Author.ID))
			return
		}

		rest := m.Content[len(c.opts.BankCommandPrefix):]
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I don't know what the bank command `%s` is.", m.Author.ID, commandName))
			return
		}

		if err := cmd(ctx, c, s, m.Message, rest); err != nil {
			glog.Errorf("Failed to run bank command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I ran into an error trying to process that bank command.", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(m.Content, c.opts.ScriptCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I'm broken right now and can't process your command.", m.Author.ID))
			return
		}

		rest := m.Content[len(c.opts.ScriptCommandPrefix):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		if commandName == "" {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I didn't understand that. Please name a command", m.Author.ID))
			return
		}

		if err := c.runScriptCommand(ctx, s, m.Message, commandName, rest); err != nil {
			glog.Errorf("Failed to run script command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I ran into an error trying to process that script command.", m.Author.ID))
			return
		}
	}

	if err := c.payForMessage(ctx, m.Message); err != nil {
		glog.Errorf("Failed to pay for message: %v", err)
	}
}

var errCommandNotFound = errors.New("command not found")

func (c *Client) resolveScriptName(ctx context.Context, commandName string) ([]byte, string, error) {
	sepIndex := strings.Index(commandName, ":")
	if sepIndex != -1 {
		// Look up via qualified name.
		encodedScriptHandle := commandName[:sepIndex]
		scriptHandle, err := base64.RawURLEncoding.DecodeString(encodedScriptHandle)
		if err != nil {
			return nil, "", errCommandNotFound
		}
		name := commandName[sepIndex+1:]
		return scriptHandle, name, nil
	} else {
		// Look up via an alias name.
		contentResp, err := c.deedsClient.GetContent(ctx, &deedspb.GetContentRequest{
			Type: "command",
			Name: commandName,
		})
		if err != nil {
			if grpc.Code(err) == codes.NotFound {
				return nil, "", errCommandNotFound
			}
			return nil, "", err
		}
		return nil, string(contentResp.Content), errCommandNotFound
	}
	return nil, "", errCommandNotFound
}

func (c *Client) runScriptCommand(ctx context.Context, s *discordgo.Session, m *discordgo.Message, commandName string, rest string) error {
	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	scriptAccountHandle, scriptName, err := c.resolveScriptName(ctx, commandName)
	if err != nil {
		if err == errCommandNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I don't know what the `%s` command is.", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	resp, err := c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		ExecutingAccountHandle: resolveResp.AccountHandle,
		ExecutingAccountKey:    resolveResp.AccountKey,
		ScriptAccountHandle:    scriptAccountHandle,
		Name:                   scriptName,
		Rest:                   rest,
		Context: &scriptspb.Context{
			BridgeName: "discord",
			Mention:    fmt.Sprintf("<@!%s>", m.Author.ID),
		},
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I couldn't find a script named `%s` owned by the account `%s`.", m.Author.ID, scriptName, base64.RawURLEncoding.EncodeToString(scriptAccountHandle)))
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, that's not a valid script name.", m.Author.ID))
			return nil
		} else if grpc.Code(err) == codes.FailedPrecondition {
			// TODO: get the script's correct billing account
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, the script's billing account (which may be you!) doesn't have enough funds to run this script.", m.Author.ID))
			return nil
		}
		return err
	}

	withdrawalDetails := make([]string, len(resp.Withdrawal))
	for i, withdrawal := range resp.Withdrawal {
		withdrawalDetails[i] = fmt.Sprintf("%d %s to `%s`", withdrawal.Amount, c.opts.CurrencyName, base64.RawURLEncoding.EncodeToString(withdrawal.TargetAccountHandle))
	}

	var usageDetails string
	switch resp.BillingMethod {
	case scriptspb.BillingMethod_BILL_EXECUTING_ACCOUNT:
		usageDetails = fmt.Sprintf("%d %s was billed as usage to <@!%s>.", resp.UsageCost, c.opts.CurrencyName, m.Author.ID)
	case scriptspb.BillingMethod_BILL_OWNING_ACCOUNT:
		usageDetails = fmt.Sprintf("%d %s was billed as usage to the script's owner.", resp.UsageCost, c.opts.CurrencyName)
	case scriptspb.BillingMethod_BILL_NOBODY:
		usageDetails = fmt.Sprintf("Nobody was billed for the use of this script.")
	default:
		usageDetails = fmt.Sprintf("I don't know how this script was billed.")
	}

	var billingDetails string
	if len(resp.Withdrawal) > 0 {
		billingDetails = fmt.Sprintf("The following withdrawals were made from your account:\n%s\n\n%s", strings.Join(withdrawalDetails, "\n"), usageDetails)
	} else {
		billingDetails = usageDetails
	}

	if resp.Ok {
		stdout := resp.Stdout
		if len(stdout) > 1500 {
			stdout = stdout[:1500]
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>:\n\n%s\n\n_%s_", m.Author.ID, stdout, billingDetails))
	} else {
		stderr := resp.Stderr
		if len(stderr) > 1500 {
			stderr = stderr[:1500]
		}
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, it looks like the script ran into an error:\n```\n%s\n```\n_%s_", m.Author.ID, stderr, billingDetails))
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
