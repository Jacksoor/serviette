package client

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/store"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var checkingAccountName = "checking"

type Options struct {
	BankCommandPrefix   string
	ScriptCommandPrefix string
	Status              string
	CurrencyName        string
}

type Client struct {
	opts *Options

	session *discordgo.Session
	store   *store.Store

	accountsClient accountspb.AccountsClient
	assetsClient   assetspb.AssetsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

func New(token string, opts *Options, store *store.Store, accountsClient accountspb.AccountsClient, assetsClient assetspb.AssetsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		opts: opts,

		session: session,
		store:   store,

		accountsClient: accountsClient,
		assetsClient:   assetsClient,
		moneyClient:    moneyClient,
		scriptsClient:  scriptsClient,
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

func (c *Client) ready(s *discordgo.Session, event *discordgo.Ready) {
	glog.Info("Discord ready.")
	s.UpdateStatus(0, c.opts.Status)
}

func (c *Client) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()

	if m.Author.ID == s.State.User.ID {
		return
	}

	fail := false
	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		glog.Errorf("Failed to ensure account: %v", err)
		fail = true
	}

	if strings.HasPrefix(m.Content, c.opts.BankCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> I'm broken right now and can't process your command.", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> I don't know what the bank command `%s` is.", m.Author.ID, commandName))
			return
		}

		if err := cmd(ctx, c, s, m.Message, rest); err != nil {
			glog.Errorf("Failed to run command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> I ran into an error trying to process that command.", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(m.Content, c.opts.ScriptCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s> I'm broken right now and can't process your command.", m.Author.ID))
			return
		}

		return
	}

	if err := c.payForMessage(ctx, m.Message); err != nil {
		glog.Errorf("Failed to pay for message: %v", err)
	}
}

func (c *Client) ensureAccount(ctx context.Context, authorID string) error {
	_, err := c.store.Account(ctx, authorID, checkingAccountName)

	if err == nil {
		return nil
	}

	if err != store.ErrNotFound {
		return err
	}

	resp, err := c.accountsClient.Create(ctx, &accountspb.CreateRequest{})
	if err != nil {
		return err
	}

	return c.store.Associate(ctx, authorID, checkingAccountName, resp.AccountHandle, resp.AccountKey)
}

func (c *Client) payForMessage(ctx context.Context, m *discordgo.Message) error {
	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		return err
	}

	checking, err := c.store.Account(ctx, m.Author.ID, checkingAccountName)
	if err != nil {
		return err
	}

	_, err = c.moneyClient.Add(ctx, &moneypb.AddRequest{
		AccountHandle: checking.Handle,
		Amount:        1,
	})
	if err != nil {
		return err
	}

	return nil
}
