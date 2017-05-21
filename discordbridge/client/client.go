package client

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/store"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	namespb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type command func(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error

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
	namesClient    namespb.NamesClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

func New(token string, opts *Options, store *store.Store, accountsClient accountspb.AccountsClient, namesClient namespb.NamesClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		opts: opts,

		session: session,
		store:   store,

		accountsClient: accountsClient,
		namesClient:    namesClient,
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I'm broken right now and can't process your command.", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I don't know what the bank command `%s` is.", m.Author.ID, commandName))
			return
		}

		if err := cmd(ctx, c, s, m.Message, rest); err != nil {
			glog.Errorf("Failed to run bank command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I ran into an error trying to process that bank command.", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(m.Content, c.opts.ScriptCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I'm broken right now and can't process your command.", m.Author.ID))
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

		if err := c.runScriptCommand(ctx, s, m.Message, commandName, rest); err != nil {
			glog.Errorf("Failed to run script command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I ran into an error trying to process that script command.", m.Author.ID))
			return
		}
	}

	if err := c.payForMessage(ctx, m.Message); err != nil {
		glog.Errorf("Failed to pay for message: %v", err)
	}
}

var errCommandNotFound = errors.New("command not found")

func (c *Client) resolveScriptName(ctx context.Context, commandName string) ([]byte, string, error) {
	sepIndex := strings.Index(commandName, "/")
	if sepIndex != -1 {
		// Look up via qualified name.
		encodedScriptHandle := commandName[:sepIndex]
		scriptHandle, err := hex.DecodeString(encodedScriptHandle)
		if err != nil {
			return nil, "", errCommandNotFound
		}
		name := commandName[sepIndex+1:]
		return scriptHandle, name, nil
	} else {
		// Look up via an alias name.
		contentResp, err := c.namesClient.GetContent(ctx, &namespb.GetContentRequest{
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
	checking, err := c.store.Account(ctx, m.Author.ID, checkingAccountName)
	if err != nil {
		return err
	}

	scriptAccountHandle, scriptName, err := c.resolveScriptName(ctx, commandName)
	if err != nil {
		if err == errCommandNotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I don't know what the `%s` command is.", m.Author.ID, commandName))
			return nil
		}
		return err
	}

	_, err = c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		ExecutingAccountHandle: checking.Handle,
		ExecutingAccountKey:    checking.Key,
		ScriptAccountHandle:    scriptAccountHandle,
		Name:                   scriptName,
		Context: &scriptspb.Context{
			BridgeName: "discord",
			Mention:    fmt.Sprintf("<@%s>", m.Author.ID),
		},
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, I couldn't find a script named `%s` owned by the account `%s`.", m.Author.ID, scriptName, hex.EncodeToString(scriptAccountHandle)))
			return nil
		} else if grpc.Code(err) == codes.FailedPrecondition {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Sorry <@%s>, you don't have enough funds in your checking account to run that command.", m.Author.ID))
			return nil
		}
		return err
	}

	return nil
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
		Amount:        c.messageEarnings(m.Content),
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) messageEarnings(content string) int64 {
	return 1
}
