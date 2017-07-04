package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/budget"

	"github.com/porpoises/kobun4/discordbridge/statsstore"
	"github.com/porpoises/kobun4/discordbridge/varstore"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Options struct {
	Status  string
	HomeURL string
}

type Client struct {
	session *discordgo.Session

	opts *Options

	discordBotsToken string
	knownGuildsOnly  bool
	rpcTarget        net.Addr

	vars     *varstore.Store
	stats    *statsstore.Store
	budgeter *budget.Budgeter

	scriptsClient scriptspb.ScriptsClient

	metaCommandRegexp *regexp.Regexp
}

func New(token string, opts *Options, discordBotsToken string, knownGuildsOnly bool, rpcTarget net.Addr, vars *varstore.Store, stats *statsstore.Store, budgeter *budget.Budgeter, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts: opts,

		discordBotsToken: discordBotsToken,
		knownGuildsOnly:  knownGuildsOnly,
		rpcTarget:        rpcTarget,

		vars:     vars,
		stats:    stats,
		budgeter: budgeter,

		scriptsClient: scriptsClient,
	}

	session.AddHandler(client.ready)
	session.AddHandler(client.guildCreate)
	session.AddHandler(client.guildDelete)
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
	guildIDs := make([]string, len(r.Guilds))
	for i, guild := range r.Guilds {
		guildIDs[i] = guild.ID
	}
	glog.Infof("Discord ready; connected guilds: %+v", guildIDs)
	s.UpdateStatus(0, c.opts.Status)
	c.metaCommandRegexp = regexp.MustCompile(fmt.Sprintf(`^<@!?%s>(.*)$`, regexp.QuoteMeta(s.State.User.ID)))
}

var defaultAdminRoleName = "Kobun Administrators"

func (c *Client) memberIsAdmin(guildVars *varstore.GuildVars, guild *discordgo.Guild, member *discordgo.Member) bool {
	if member == nil || guild == nil {
		return false
	}

	if guildVars.AdminRoleID != nil {
		// Check if they have the admin role ID.
		for _, roleID := range member.Roles {
			if roleID == *guildVars.AdminRoleID {
				return true
			}
		}
	} else {
		// Look up role by name, or if they have Administrator.
		adminRoleIDs := make([]string, 0)
		for _, role := range guild.Roles {
			if role.Name == defaultAdminRoleName || (role.Permissions&discordgo.PermissionAdministrator != 0) {
				adminRoleIDs = append(adminRoleIDs, role.ID)
			}
		}

		for _, roleID := range member.Roles {
			for _, adminRoleID := range adminRoleIDs {
				if roleID == adminRoleID {
					return true
				}
			}
		}
	}

	return false
}

func (c *Client) updateDiscordBotsServerCount(ctx context.Context) error {
	if c.discordBotsToken == "" {
		return nil
	}

	raw, err := json.Marshal(struct {
		ShardID     int `json:"shard_id"`
		ShardCount  int `json:"shard_count"`
		ServerCount int `json:"server_count"`
	}{
		c.session.ShardID,
		c.session.ShardCount,
		len(c.session.State.Guilds),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://bots.discord.pw/api/bots/%s/stats", c.session.State.User.ID), bytes.NewBuffer(raw))
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", c.discordBotsToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to update bot stats: %d %s", resp.StatusCode, resp.Status)
	}

	return nil
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
			if err == varstore.ErrNotFound && !c.knownGuildsOnly {
				glog.Infof("No guild vars for %s, creating", m.Guild.ID)

				if err := c.vars.CreateGuildVars(ctx, tx, m.Guild.ID); err != nil {
					return err
				}

				guildVars, err = c.vars.GuildVars(ctx, tx, m.Guild.ID)
				if err != nil {
					return err
				}
				tx.Commit()

				return nil
			}
			return err
		}

		return nil
	}(); err != nil {
		if err == varstore.ErrNotFound && c.knownGuildsOnly {
			glog.Warningf("No guild vars found for %s, leaving.", m.Guild.ID)
			s.GuildLeave(m.Guild.ID)
			return
		}

		panic(fmt.Sprintf("Failed to get guild vars: %v", err))
	}

	glog.Infof("Guild vars for %s: %+v", m.Guild.ID, guildVars)

	if err := c.updateDiscordBotsServerCount(ctx); err != nil {
		glog.Errorf("Failed to update server count on bots.discord.pw: %v", err)
	}
}

func (c *Client) guildDelete(s *discordgo.Session, m *discordgo.GuildDelete) {
	ctx := context.Background()

	tx, err := c.vars.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return
	}
	defer tx.Rollback()

	if err := c.vars.SetGuildVars(ctx, tx, m.Guild.ID, nil); err != nil {
		glog.Errorf("Failed to clear guild vars: %v", err)
		return
	}

	tx.Commit()

	glog.Infof("Cleared guild vars for %s", m.Guild.ID)

	if err := c.updateDiscordBotsServerCount(ctx); err != nil {
		glog.Errorf("Failed to update server count on bots.discord.pw: %v", err)
	}
}

var privateGuildVars = &varstore.GuildVars{
	ScriptCommandPrefix: "",
	Quiet:               false,
}

type errorStatus int

const (
	errorStatusInternal = iota
	errorStatusNoise
	errorStatusScript
	errorStatusUser
	errorStatusUnauthorized
	errorStatusRecoverable
	errorStatusRateLimited
)

var errorSigils = map[errorStatus]string{
	errorStatusInternal:     "‼",
	errorStatusNoise:        "❎",
	errorStatusScript:       "❗",
	errorStatusUser:         "❎",
	errorStatusUnauthorized: "🚫",
	errorStatusRecoverable:  "⚠",
	errorStatusRateLimited:  "🐢",
}

type commandError struct {
	status  errorStatus
	note    string
	details string
}

func (c *commandError) Error() string {
	return c.note
}

func (c *Client) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()

	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Author.Bot {
		return
	}

	var guild *discordgo.Guild

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

			guild, err = s.Guild(channel.GuildID)
			if err != nil {
				return err
			}

			guildVars, err = c.vars.GuildVars(ctx, tx, channel.GuildID)
			if err != nil {
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
	}

	if err := c.handleMessage(ctx, guildVars, m.Message, guild, channel, member, content); err != nil {
		cErr, ok := err.(*commandError)
		if !ok {
			glog.Errorf("Error handling message: %v", err)
			cErr = &commandError{
				status: errorStatusInternal,
				note:   "Internal error",
			}
		}

		if cErr.status == errorStatusNoise && guildVars.Quiet {
			return
		}

		messageSend := &discordgo.MessageSend{
			Content: fmt.Sprintf("<@%s>: **%s %s**", m.Author.ID, errorSigils[cErr.status], cErr.note),
		}

		if cErr.details != "" {
			messageSend.Embed = &discordgo.MessageEmbed{
				Color:       0xb50000,
				Description: cErr.details,
			}
		}

		msg, err := s.ChannelMessageSendComplex(channel.ID, messageSend)
		if err != nil {
			glog.Errorf("Failed to send error message: %v", err)
			return
		}

		if guildVars.DeleteErrorsAfter > 0 {
			go func() {
				<-time.After(guildVars.DeleteErrorsAfter)
				if err := s.ChannelMessageDelete(channel.ID, msg.ID); err != nil {
					glog.Error("Failed to delete error message: %v", err)
				}
			}()
		}
	}
}

func (c *Client) handleMessage(ctx context.Context, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, content string) error {
	if member != nil {
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
				return &commandError{
					status: errorStatusNoise,
					note:   fmt.Sprintf("Meta command `%s` not found", commandName),
				}
			}

			return cmd(ctx, c, guildVars, m, guild, channel, member, rest)
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

		return c.runScriptCommand(ctx, guildVars, m, guild, channel, member, commandName, rest)
	}

	if err := c.stats.RecordUserChannelMessage(ctx, m.Author.ID, channel.ID, int64(len(m.Content))); err != nil {
		glog.Errorf("Failed to record stats: %v", err)
	}

	return nil
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

func (c *Client) runScriptCommand(ctx context.Context, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, commandName string, rest string) error {
	linked := commandNameIsLinked(commandName)
	if member != nil && !linked && !guildVars.AllowUnprivilegedUnlinkedCommands && !c.memberIsAdmin(guildVars, guild, member) {
		return &commandError{
			status: errorStatusNoise,
			note:   "Only the server's Kobun administrators can run unlinked commands",
		}
	}

	ownerName, scriptName, err := resolveScriptName(ctx, c, channel.GuildID, commandName)
	if err != nil {
		switch err {
		case errNotFound:
			return &commandError{
				status: errorStatusNoise,
				note:   fmt.Sprintf("Command `%s%s` not found", guildVars.ScriptCommandPrefix, commandName),
			}
		}
		return err
	}

	metaResp, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: ownerName,
		Name:      scriptName,
	})
	if err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			if linked {
				return &commandError{
					status: errorStatusScript,
					note:   "Command link references non-existent script",
				}
			}
			return &commandError{
				status: errorStatusNoise,
				note:   fmt.Sprintf("Command `%s%s/%s` not found", guildVars.ScriptCommandPrefix, ownerName, scriptName),
			}

		case codes.Unavailable:
			return &commandError{
				status: errorStatusRecoverable,
				note:   "Currently unavailable, please try again later",
			}
		default:
			return err
		}
	}
	if !metaResp.Meta.Published {
		if err := c.vars.CanRunUnpublishedScript(ctx, m.Author.ID, ownerName); err != nil {
			if err != varstore.ErrNotPermitted {
				return err
			}

			if linked {
				return &commandError{
					status: errorStatusScript,
					note:   "Command link references non-existent script",
				}
			}
			return &commandError{
				status: errorStatusNoise,
				note:   fmt.Sprintf("Command `%s%s/%s` not found", guildVars.ScriptCommandPrefix, ownerName, scriptName),
			}
		}
	}

	remainingBudget, err := c.budgeter.Remaining(ctx, m.Author.ID)
	if err != nil {
		return err
	}

	if remainingBudget <= 0 {
		return &commandError{
			status: errorStatusRateLimited,
			note:   "Too many commands, please slow down",
		}
	}

	c.session.ChannelTyping(m.ChannelID)
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
				return &commandError{
					status: errorStatusScript,
					note:   "Command link references non-existent script",
				}
			}
			return &commandError{
				status: errorStatusNoise,
				note:   fmt.Sprintf("Command `%s%s/%s` not found", guildVars.ScriptCommandPrefix, ownerName, scriptName),
			}
		case codes.Unavailable:
			return &commandError{
				status: errorStatusRecoverable,
				note:   "Currently unavailable, please try again later",
			}
		default:
			return err
		}
	}

	if err := c.budgeter.Charge(ctx, m.Author.ID, time.Duration(resp.Result.Timings.RealNanos)*time.Nanosecond); err != nil {
		return err
	}

	channelID := m.ChannelID
	if resp.Result.OutputParams.Private {
		channel, err := c.session.UserChannelCreate(m.Author.ID)
		if err != nil {
			return err
		}
		channelID = channel.ID
	}

	waitStatus := syscall.WaitStatus(resp.Result.WaitStatus)

	if waitStatus.ExitStatus() == 0 || (waitStatus.ExitStatus() == 2 && len(resp.Stdout) > 0) {
		outputFormatter, ok := OutputFormatters[resp.Result.OutputParams.Format]
		if !ok {
			return &commandError{
				status: errorStatusScript,
				note:   fmt.Sprintf("Output format `%s` unknown", resp.Result.OutputParams.Format),
			}
		}

		if len(resp.Stdout) == 0 {
			return nil
		}

		execOK := waitStatus.ExitStatus() == 0
		messageSend, err := outputFormatter(m.Author.ID, resp.Stdout, execOK)
		if err != nil {
			if iErr, ok := err.(invalidOutputError); ok {
				return &commandError{
					status:  errorStatusScript,
					note:    fmt.Sprintf("Output format `%s` unknown", resp.Result.OutputParams.Format),
					details: fmt.Sprintf("```%s```", iErr.Error()),
				}
			}
			return err
		}

		msg, err := c.session.ChannelMessageSendComplex(channelID, messageSend)
		if err != nil {
			return err
		}

		if !execOK && guildVars.DeleteErrorsAfter > 0 {
			go func() {
				<-time.After(guildVars.DeleteErrorsAfter)
				if err := c.session.ChannelMessageDelete(channel.ID, msg.ID); err != nil {
					glog.Error("Failed to delete error message: %v", err)
				}
			}()
		}

		return nil
	} else if resp.Result.TimeLimitExceeded {
		return &commandError{
			status: errorStatusScript,
			note:   "Script took too long!",
		}
	} else if waitStatus.Signaled() {
		return &commandError{
			status: errorStatusScript,
			note:   fmt.Sprintf("Script was killed by signal %d (%s)!", waitStatus.Signal(), waitStatus.Signal()),
		}
	} else {
		stderr := resp.Stderr
		if len(stderr) > 1500 {
			stderr = stderr[:1500]
		}

		var details string
		if len(stderr) > 0 {
			details = fmt.Sprintf("```%s```", string(stderr))
		} else {
			details = "(stderr was empty)"
		}

		return &commandError{
			status:  errorStatusScript,
			note:    "Error occurred!",
			details: details,
		}
	}
}
