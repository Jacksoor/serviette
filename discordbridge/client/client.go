package client

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
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

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Options struct {
	Status                 string
	HomeURL                string
	ChangelogChannelID     string
	StatsReportingInterval time.Duration
	StatsReporterTargets   map[string]string
	MinCostPerUser         time.Duration
}

type Client struct {
	session *discordgo.Session

	opts *Options

	knownGuildsOnly bool
	rpcTarget       net.Addr

	vars     *varstore.Store
	stats    *statsstore.Store
	budgeter *budget.Budgeter

	accountsClient accountspb.AccountsClient
	scriptsClient  scriptspb.ScriptsClient

	metaCommandRegexp *regexp.Regexp
}

func New(token string, opts *Options, knownGuildsOnly bool, rpcTarget net.Addr, vars *varstore.Store, stats *statsstore.Store, budgeter *budget.Budgeter, accountsClient accountspb.AccountsClient, scriptsClient scriptspb.ScriptsClient) (*Client, error) {
	session, err := discordgo.New(fmt.Sprintf("Bot %s", token))
	if err != nil {
		return nil, err
	}

	client := &Client{
		session: session,

		opts: opts,

		knownGuildsOnly: knownGuildsOnly,
		rpcTarget:       rpcTarget,

		vars:     vars,
		stats:    stats,
		budgeter: budgeter,

		accountsClient: accountsClient,
		scriptsClient:  scriptsClient,
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
	c.metaCommandRegexp = regexp.MustCompile(fmt.Sprintf(`^<@!?%s>((?s).*)$`, regexp.QuoteMeta(s.State.User.ID)))

	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(c.opts.StatsReportingInterval)

		for {
			<-ticker.C
			c.reportStats(ctx)
		}
	}()
}

var defaultAdminRoleNames = []string{"Kobun Administrators", "Kobun Administrator"}

func (c *Client) memberIsAdmin(guildVars *varstore.GuildVars, guild *discordgo.Guild, member *discordgo.Member) bool {
	if member == nil || guild == nil {
		return false
	}

	// Check if they are the owner, look up role by name, or if they have Administrator.
	if member.User.ID == guild.OwnerID {
		return true
	}

	adminRoleIDs := make([]string, 0)
	for _, role := range guild.Roles {
		if role.Permissions&discordgo.PermissionAdministrator != 0 {
			adminRoleIDs = append(adminRoleIDs, role.ID)
			continue
		}

		for _, defaultAdminRoleName := range defaultAdminRoleNames {
			if strings.EqualFold(role.Name, defaultAdminRoleName) {
				adminRoleIDs = append(adminRoleIDs, role.ID)
			}
		}
	}

	for _, roleID := range member.Roles {
		for _, adminRoleID := range adminRoleIDs {
			if roleID == adminRoleID {
				return true
			}
		}
	}

	return false
}

func (c *Client) reportStats(ctx context.Context) {
	var g sync.WaitGroup

	for provider, token := range c.opts.StatsReporterTargets {
		statsReporter, ok := statsReporters[provider]
		if !ok {
			glog.Errorf("No stats reporter for provider: %s", provider)
			continue
		}

		g.Add(1)
		go func(provider string, token string) {
			defer g.Done()

			ctx := context.WithValue(ctx, statsAuthTokenContextKey(provider), token)

			serverCount := len(c.session.State.Guilds)
			glog.Infof("Reporting stats to %s: shard ID = %d, shard count = %d, server count = %d", provider, c.session.ShardID, c.session.ShardCount, serverCount)

			if err := statsReporter(ctx, c.session.State.User.ID, c.session.ShardID, c.session.ShardCount, serverCount); err != nil {
				glog.Errorf("Failed to report stats to %s: %v", provider, err)
			} else {
				glog.Infof("Successfully reported stats to %s", provider)
			}
		}(provider, token)
	}

	g.Wait()
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

				c.sendToChangelog(fmt.Sprintf("Added to guild **%s** (%s).", m.Guild.Name, m.Guild.ID))
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

	glog.Infof("Guild vars for %s (%s): %+v", m.Guild.ID, m.Guild.Name, guildVars)
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

	c.sendToChangelog(fmt.Sprintf("Removed from guild **%s** (%s).", m.Guild.Name, m.Guild.ID))

	glog.Infof("Cleared guild vars for %s", m.Guild.ID)
}

func (c *Client) sendToChangelog(msg string) {
	if c.opts.ChangelogChannelID == "" {
		return
	}

	c.session.ChannelMessageSend(c.opts.ChangelogChannelID, msg)
}

var privateGuildVars = &varstore.GuildVars{
	Quiet: false,
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

	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		glog.Errorf("Failed to get channel: %v", err)
		return
	}

	var guildVars *varstore.GuildVars
	if channel.Type == discordgo.ChannelTypeDM {
		guildVars = privateGuildVars
	} else {
		if err := func() error {
			tx, err := c.vars.BeginTx(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			guild, err = s.State.Guild(channel.GuildID)
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
		member, err = c.session.State.Member(channel.GuildID, m.Author.ID)
		if err != nil {
			glog.Errorf("Failed to get member: %v", err)
			return
		}
	}

	if err := c.handleMessage(ctx, guildVars, m.Message, guild, channel, member, content); err != nil {
		c.sendErrorMessage(err, guildVars, m.Message, channel)
	}
}

func (c *Client) sendErrorMessage(err error, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel) error {
	cErr, ok := err.(*commandError)
	if !ok {
		glog.Errorf("Error handling message: %v", err)
		cErr = &commandError{
			status: errorStatusInternal,
			note:   "Internal error",
		}
	}

	if cErr.status == errorStatusNoise && guildVars.Quiet {
		return nil
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

	msg, err := c.session.ChannelMessageSendComplex(channel.ID, messageSend)
	if err != nil {
		glog.Errorf("Failed to send error message: %v", err)
		return err
	}

	if guildVars.MessageExpiry > 0 {
		go func() {
			<-time.After(guildVars.MessageExpiry)
			if err := c.session.ChannelMessageDelete(channel.ID, msg.ID); err != nil {
				glog.Error("Failed to delete error message: %v", err)
			}
		}()
	}

	return nil
}

func (c *Client) handleMessage(ctx context.Context, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, content string) error {
	if member != nil {
		if match := c.metaCommandRegexp.FindStringSubmatch(content); match != nil {
			lines := strings.Split(match[1], "\n")

			for _, line := range lines {
				line = strings.TrimSpace(line)
				firstSpaceIndex := strings.Index(line, " ")

				var commandName string
				var rest string

				if firstSpaceIndex == -1 {
					commandName = line
					rest = ""
				} else {
					commandName = line[:firstSpaceIndex]
					rest = strings.TrimSpace(line[firstSpaceIndex+1:])
				}

				var cmd metaCommand
				var ok bool
				if commandName == "" {
					cmd, ok = metaCommands["help"]
				} else {
					cmd, ok = metaCommands[commandName]
				}

				if !ok {
					c.sendErrorMessage(&commandError{
						status: errorStatusUser,
						note:   fmt.Sprintf("Meta command `%s` not found", commandName),
					}, guildVars, m, channel)
					continue
				}

				if err := cmd(ctx, c, guildVars, m, guild, channel, member, rest); err != nil {
					c.sendErrorMessage(err, guildVars, m, channel)
					continue
				}
			}

			return nil
		}
	}

	if guild != nil {
		var commandName string
		var link *varstore.Link

		if err := func() error {
			tx, err := c.vars.BeginTx(ctx)
			if err != nil {
				return err
			}
			defer tx.Rollback()

			commandName, link, err = c.vars.FindLink(ctx, tx, guild.ID, content)
			return err
		}(); err != nil {
			if err != varstore.ErrNotFound {
				glog.Errorf("Failed to look up command from database: %v", err)
				return nil
			}
		}

		if link != nil {
			rest := strings.TrimSpace(m.Content[len(commandName):])
			return c.runScriptCommand(ctx, guildVars, m, guild, channel, member, commandName, link, rest)
		}

		if err := c.stats.RecordUserChannelMessage(ctx, m.Author.ID, channel.ID, int64(len(m.Content))); err != nil {
			glog.Errorf("Failed to record stats: %v", err)
		}
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

func (c *Client) runScriptCommand(ctx context.Context, guildVars *varstore.GuildVars, m *discordgo.Message, guild *discordgo.Guild, channel *discordgo.Channel, member *discordgo.Member, commandName string, link *varstore.Link, rest string) error {
	metaResp, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: link.OwnerName,
		Name:      link.ScriptName,
	})
	if err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			return &commandError{
				status: errorStatusScript,
				note:   "References non-existent script",
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
	if metaResp.Meta.Visibility == scriptspb.Visibility_UNPUBLISHED {
		if _, err := c.accountsClient.CheckAccountIdentifier(ctx, &accountspb.CheckAccountIdentifierRequest{
			Username:   link.OwnerName,
			Identifier: fmt.Sprintf("discord/%s", m.Author.ID),
		}); err != nil {
			switch grpc.Code(err) {
			case codes.NotFound:
				return &commandError{
					status: errorStatusScript,
					note:   "References non-existent script",
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

	if err := c.budgeter.Charge(ctx, m.Author.ID, c.opts.MinCostPerUser); err != nil {
		return err
	}

	c.session.ChannelTyping(m.ChannelID)
	resp, err := c.scriptsClient.Execute(ctx, &scriptspb.ExecuteRequest{
		OwnerName: link.OwnerName,
		Name:      link.ScriptName,
		Stdin:     []byte(rest),
		Context: &scriptspb.Context{
			BridgeName:  "discord",
			CommandName: commandName,

			UserId:    m.Author.ID,
			ChannelId: m.ChannelID,
			GroupId:   channel.GuildID,
			NetworkId: "discord",

			InputMessageId: m.ID,
		},
		BridgeTarget: c.rpcTarget.String(),
	})
	if err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			return &commandError{
				status: errorStatusScript,
				note:   "References non-existent script",
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

	totalCost := time.Duration(resp.Result.Timings.RealNanos) * time.Nanosecond
	remainingCost := totalCost - c.opts.MinCostPerUser

	if remainingCost > 0 {
		if err := c.budgeter.Charge(ctx, m.Author.ID, remainingCost); err != nil {
			return err
		}
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

		if (!execOK || resp.Result.OutputParams.Expires) && guildVars.MessageExpiry > 0 {
			go func() {
				<-time.After(guildVars.MessageExpiry)
				if err := c.session.ChannelMessageDelete(channel.ID, msg.ID); err != nil {
					glog.Errorf("Failed to delete message: %v", err)
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
