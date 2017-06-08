package client

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net/mail"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type command func(ctx context.Context, c *Client, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, rest string) error

type Options struct {
	Status string
	WebURL string
}

type Client struct {
	session *discordgo.Session

	opts *Options

	networkInfoServiceTarget string

	vars *varstore.Store

	accountsClient accountspb.AccountsClient
	moneyClient    moneypb.MoneyClient
	scriptsClient  scriptspb.ScriptsClient
}

func New(token string, opts *Options, networkInfoServiceTarget string, vars *varstore.Store, accountsClient accountspb.AccountsClient, moneyClient moneypb.MoneyClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts: opts,

		networkInfoServiceTarget: networkInfoServiceTarget,

		vars: vars,

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

var GuildVars *varstore.GuildVars = &varstore.GuildVars{
	ScriptCommandPrefix: ".",
	BankCommandPrefix:   "$",
	CurrencyName:        "coins",
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

	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		glog.Errorf("Failed to ensure account: %v", err)
		fail = true
	}

	if strings.HasPrefix(m.Content, guildVars.BankCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		rest := m.Content[len(guildVars.BankCommandPrefix):]
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
			if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s` not found**", m.Author.ID, guildVars.BankCommandPrefix, commandName))
			}
			return
		}

		if err := cmd(ctx, c, guildVars, s, m.Message, channel, rest); err != nil {
			glog.Errorf("Failed to run bank command %s: %v", commandName, err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if strings.HasPrefix(m.Content, guildVars.ScriptCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		return
	}

	if err := c.payForMessage(ctx, m.Message, channel); err != nil {
		glog.Errorf("Failed to pay for message: %v", err)
	}
}

type outputFormatter func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error)

func copyPart(dest io.Writer, part *multipart.Part) (int64, error) {
	encoding := part.Header.Get("Content-Transfer-Encoding")

	switch encoding {
	case "base64":
		dec := base64.NewDecoder(base64.StdEncoding, part)
		return io.Copy(dest, dec)
	case "":
		return io.Copy(dest, part)
	}

	return 0, fmt.Errorf("unknown Content-Transfer-Encoding: %s", encoding)
}

type invalidOutputError struct {
	error
}

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

		if err := json.Unmarshal(r.Stdout, embed); err != nil {
			return nil, invalidOutputError{err}
		}

		return &discordgo.MessageSend{
			Embed: embed,
		}, nil
	},

	"discord.embed_multipart": func(r *scriptspb.ExecuteResponse) (*discordgo.MessageSend, error) {
		embed := &discordgo.MessageEmbed{}

		msg, err := mail.ReadMessage(bytes.NewReader(r.Stdout))
		if err != nil {
			return nil, invalidOutputError{err}
		}

		mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
		if err != nil {
			return nil, invalidOutputError{err}
		}

		if !strings.HasPrefix(mediaType, "multipart/") {
			return nil, invalidOutputError{err}
		}

		boundary, ok := params["boundary"]
		if !ok {
			return nil, invalidOutputError{errors.New("boundary not found in multipart header")}
		}

		mr := multipart.NewReader(msg.Body, boundary)

		// Parse the payload (first part).
		payloadPart, err := mr.NextPart()
		if err != nil {
			return nil, invalidOutputError{err}
		}

		// Decode the embed.
		var payloadBuf bytes.Buffer
		if _, err := copyPart(&payloadBuf, payloadPart); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(payloadBuf.Bytes(), embed); err != nil {
			return nil, invalidOutputError{err}
		}

		// Decode all the files.
		files := make([]*discordgo.File, 0)
		for {
			filePart, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, invalidOutputError{err}
			}

			buf := new(bytes.Buffer)
			if _, err := copyPart(buf, filePart); err != nil {
				return nil, err
			}

			files = append(files, &discordgo.File{
				Name:        filePart.FileName(),
				ContentType: filePart.Header.Get("Content-Type"),
				Reader:      buf,
			})
		}

		return &discordgo.MessageSend{
			Embed: embed,
			Files: files,
		}, nil
	},
}

func (c *Client) prettyBillingDetails(commandName string, requirements *scriptspb.Requirements, guildVars *varstore.GuildVars, r *scriptspb.ExecuteResponse) string {
	parts := []string{}

	if !requirements.BillUsageToOwner && r.UsageCost > 0 {
		parts = append(parts, fmt.Sprintf("**Usage cost:** %d %s", r.UsageCost, guildVars.CurrencyName))
	} else {
		// Leave top line empty.
		parts = append(parts, "")
	}

	charges := make([]string, len(r.Charge))
	for i, withdrawal := range r.Charge {
		charges[i] = fmt.Sprintf("%d %s to `%s`", withdrawal.Amount, guildVars.CurrencyName, base64.RawURLEncoding.EncodeToString(withdrawal.TargetAccountHandle))
	}

	if len(r.Charge) > 0 {
		parts = append(parts, fmt.Sprintf("ℹ **Charges:**\n%s", strings.Join(charges, "\n")))
	}

	return strings.Join(parts, "\n\n")
}

func (c *Client) runScriptCommand(ctx context.Context, guildVars *varstore.GuildVars, s *discordgo.Session, m *discordgo.Message, channel *discordgo.Channel, commandName string, rest string) error {
	var executingAccountHandle []byte

	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		userVars, err := c.vars.UserVars(ctx, tx, m.Author.ID)
		if err != nil {
			return false, err
		}

		executingAccountHandle = userVars.AccountHandle

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	scriptAccountHandle, scriptName, aliased, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
	if err != nil {
		switch err {
		case errNotFound:
			if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, commandName))
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
			} else if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, base64.RawURLEncoding.EncodeToString(scriptAccountHandle), scriptName))
			}
			return nil
		} else if grpc.Code(err) == codes.InvalidArgument {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❎ **Invalid command name**", m.Author.ID))
			return nil
		}
		return err
	}
	requirements := getRequirementsResp.Requirements

	var escrowedFunds int64
	if requirements.NeedsEscrow {
		parts := strings.SplitN(rest, " ", 2)
		if parts[0] == "" {
			escrowedFunds = 0
		} else {
			escrowedFunds, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				if _, ok := err.(*strconv.NumError); !ok {
					return err
				}
				escrowedFunds = 0
			} else {
				if len(parts) == 1 {
					rest = ""
				} else {
					rest = parts[1]
				}
			}
		}
	}

	waitMsg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ⌛ **Please wait, running your command...**", m.Author.ID))
	if err != nil {
		return err
	}
	defer s.ChannelMessageDelete(m.ChannelID, waitMsg.ID)

	resp, err := c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		ExecutingAccountHandle: executingAccountHandle,
		ScriptAccountHandle:    scriptAccountHandle,
		Name:                   scriptName,
		Rest:                   rest,
		Context: &scriptspb.Context{
			BridgeName:  "discord",
			CommandName: commandName,

			UserId:    m.Author.ID,
			ChannelId: m.ChannelID,
			GroupId:   channel.GuildID,
			NetworkId: "discord",

			CurrencyName:        guildVars.CurrencyName,
			ScriptCommandPrefix: guildVars.ScriptCommandPrefix,
			BankCommandPrefix:   guildVars.BankCommandPrefix,
		},
		NetworkInfoServiceTarget: c.networkInfoServiceTarget,
		EscrowedFunds:            escrowedFunds,
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
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Output format `%s` unknown!** %s", m.Author.ID, resp.OutputFormat, c.prettyBillingDetails(commandName, requirements, guildVars, resp)))
			return nil
		}

		messageSend, err := outputFormatter(resp)
		if err != nil {
			if iErr, ok := err.(invalidOutputError); ok {
				s.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
					Content: fmt.Sprintf("<@!%s>: ❗ **Command output was invalid!** %s", m.Author.ID, c.prettyBillingDetails(commandName, requirements, guildVars, resp)),
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

		messageSend.Content = fmt.Sprintf("<@!%s>: %s %s", m.Author.ID, sigil, c.prettyBillingDetails(commandName, requirements, guildVars, resp))

		if _, err := s.ChannelMessageSendComplex(m.ChannelID, messageSend); err != nil {
			return err
		}
		return nil
	} else if waitStatus.Signaled() {
		sig := waitStatus.Signal()
		switch sig {
		case syscall.SIGKILL:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Took too long!** %s", m.Author.ID, c.prettyBillingDetails(commandName, requirements, guildVars, resp)))
		default:
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@!%s>: ❗ **Script was killed by %s!** %s", m.Author.ID, sig.String(), c.prettyBillingDetails(commandName, requirements, guildVars, resp)))
		}
	} else {
		stderr := resp.Stderr
		if len(stderr) > 1500 {
			stderr = stderr[:1500]
		}

		billingDetails := c.prettyBillingDetails(commandName, requirements, guildVars, resp)
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
	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = c.vars.UserVars(ctx, tx, authorID)

	if err == nil {
		return nil
	}

	if err != varstore.ErrNotFound {
		return err
	}

	var resp *accountspb.CreateResponse
	for {
		resp, err = c.accountsClient.Create(ctx, &accountspb.CreateRequest{})
		if err != nil {
			return err
		}

		if grpc.Code(err) != codes.Unavailable {
			break
		}

		glog.Warningf("Temporary failure to ensure account: %v", err)
		time.Sleep(1 * time.Second)
	}

	if err != nil {
		return err
	}

	if err := c.vars.SetUserVars(ctx, tx, authorID, &varstore.UserVars{
		AccountHandle: resp.AccountHandle,
	}); err != nil {
		return err
	}

	tx.Commit()

	return nil
}

func (c *Client) payForMessage(ctx context.Context, m *discordgo.Message, channel *discordgo.Channel) error {
	if channel.IsPrivate {
		return nil
	}

	if err := c.ensureAccount(ctx, m.Author.ID); err != nil {
		return err
	}

	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	userVars, err := c.vars.UserVars(ctx, tx, m.Author.ID)
	if err != nil {
		return err
	}

	now := time.Now()

	channelVars, err := c.vars.ChannelVars(ctx, tx, channel.ID)
	if err != nil {
		if err != varstore.ErrNotFound {
			glog.Errorf("Failed to load channel vars: %v", err)
		}
		return nil
	}

	if userVars.LastPayoutTime.Add(channelVars.Cooldown).After(now) {
		return nil
	}

	interval := channelVars.MaxPayout - channelVars.MinPayout
	if interval > 0 {
		r, err := rand.Int(rand.Reader, big.NewInt(interval))
		if err != nil {
			glog.Errorf("Failed to generate earnings amount: %v", err)
			return nil
		}
		interval = r.Int64()
	}

	earnings := channelVars.MinPayout + interval
	if earnings == 0 {
		return nil
	}

	if _, err := c.moneyClient.Add(ctx, &moneypb.AddRequest{
		AccountHandle: userVars.AccountHandle,
		Amount:        earnings,
	}); err != nil {
		return err
	}

	userVars.LastPayoutTime = now

	if err := c.vars.SetUserVars(ctx, tx, m.Author.ID, userVars); err != nil {
		return err
	}

	tx.Commit()

	return nil
}
