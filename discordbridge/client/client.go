package client

import (
	"encoding/base64"
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

func (c *Client) runScriptCommand(ctx context.Context, s *discordgo.Session, m *discordgo.Message, commandName string, rest string) error {
	resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
		Name: aliasName(m.Author.ID),
	})
	if err != nil {
		return err
	}

	scriptAccountHandle, scriptName, err := resolveScriptName(ctx, c, commandName)
	if err != nil {
		switch err {
		case errNotFound:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I don't know what the `%s` command is.", m.Author.ID, commandName))
			return nil
		case errMisconfiguredCommand:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, the owner of the `%s` command hasn't configured their command correctly.", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	getRequestedCapsResp, err := c.scriptsClient.GetRequestedCapabilities(ctx, &scriptspb.GetRequestedCapabilitiesRequest{
		AccountHandle: scriptAccountHandle,
		Name:          scriptName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, I couldn't find a command named `%s` owned by the account `%s`.", m.Author.ID, scriptName, base64.RawURLEncoding.EncodeToString(scriptAccountHandle)))
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, that's not a valid command name.", m.Author.ID))
			return nil
		}
		return err
	}

	getAccountCapsResp, err := c.scriptsClient.GetAccountCapabilities(ctx, &scriptspb.GetAccountCapabilitiesRequest{
		ExecutingAccountHandle: resolveResp.AccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		ScriptName:             scriptName,
	})
	if err != nil {
		return err
	}

	prettyCaps := make([]string, 0)
	capSettings := make([]string, 0)
	if getRequestedCapsResp.Capabilities.BillUsageToExecutingAccount {
		if !getAccountCapsResp.Capabilities.BillUsageToExecutingAccount {
			prettyCaps = append(prettyCaps, " - "+explainBillUsageToExecutingAccount(c)+" (you!)")
		}
		capSettings = append(capSettings, "bill_usage_to_executing_account:true")
	}

	if getRequestedCapsResp.Capabilities.WithdrawalLimit > 0 {
		if getAccountCapsResp.Capabilities.WithdrawalLimit <= 0 {
			prettyCaps = append(prettyCaps, " - "+explainWithdrawalLimit(c, getRequestedCapsResp.Capabilities.WithdrawalLimit)+" (you may set this lower)")
			capSettings = append(capSettings, fmt.Sprintf("withdrawal_limit:%d", getRequestedCapsResp.Capabilities.WithdrawalLimit))
		} else {
			capSettings = append(capSettings, fmt.Sprintf("withdrawal_limit:%d", getAccountCapsResp.Capabilities.WithdrawalLimit))
		}
	}

	if len(prettyCaps) > 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf(`Sorry <@!%s>, the `+"`"+`%s`+"`"+` command requires your following additional capabilities:

%s

**If you want to allow this, please run the following command:**
`+"```"+`
$setcaps %s %s
`+"```"+`
If you have your granted capabilities to this command before, **it has been changed from the last time you ran it.**`,
			m.Author.ID, commandName, strings.Join(prettyCaps, "\n"), commandName, strings.Join(capSettings, " ")))
		return nil
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
		if grpc.Code(err) == codes.FailedPrecondition {
			if getRequestedCapsResp.Capabilities.BillUsageToExecutingAccount {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, the command's billing account doesn't have enough funds to run this command.", m.Author.ID))
			} else {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@!%s>, you don't have enough funds to run this command.", m.Author.ID))
			}
			return nil
		}
		return err
	}

	withdrawalDetails := make([]string, len(resp.Withdrawal))
	for i, withdrawal := range resp.Withdrawal {
		withdrawalDetails[i] = fmt.Sprintf("%d %s to `%s`", withdrawal.Amount, c.opts.CurrencyName, base64.RawURLEncoding.EncodeToString(withdrawal.TargetAccountHandle))
	}

	var usageDetails string
	if getRequestedCapsResp.Capabilities.BillUsageToExecutingAccount {
		usageDetails = fmt.Sprintf("%d %s was billed as usage to <@!%s>.", resp.UsageCost, c.opts.CurrencyName, m.Author.ID)
	} else {
		usageDetails = fmt.Sprintf("%d %s was billed as usage to the command's owner.", resp.UsageCost, c.opts.CurrencyName)
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
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>:\n%s\n_%s_", m.Author.ID, stdout, billingDetails))
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
