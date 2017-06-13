package client

import (
	"fmt"
	"sort"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/sync/errgroup"
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
}

var adminCommandPrefix = "kobun$"

func memberIsAdmin(adminRoleID string, member *discordgo.Member) bool {
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
		glog.Warningf("No guild vars found for %s, leaving.", m.Guild.ID)
		s.GuildLeave(m.Guild.ID)
	} else {
		glog.Infof("Guild vars for %s: %+v", m.Guild.ID, guildVars)
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
		return
	}

	var guildVars *varstore.GuildVars
	if err := func() error {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		guildVars, err = c.vars.GuildVars(ctx, tx, channel.GuildID)
		if err != nil {
			return err
		}

		return nil
	}(); err != nil {
		glog.Errorf("Failed to get guild vars: %v", err)
	}

	content := strings.TrimSpace(m.Content)

	if (content == fmt.Sprintf("<@!%s> help", s.State.User.ID) || content == fmt.Sprintf("<@%s> help", s.State.User.ID)) && channel.GuildID != "" {
		if err := showHelp(ctx, c, guildVars, m.Message, channel); err != nil {
			glog.Errorf("Failed to run help command: %v", err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
		}
		return
	}

	if strings.HasPrefix(content, adminCommandPrefix) {
		if fail {
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		member, err := c.session.GuildMember(channel.GuildID, m.Author.ID)
		if err != nil {
			glog.Errorf("Failed to get member: %v", err)
			s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ‼ **Internal error**", m.Author.ID))
			return
		}

		if !memberIsAdmin(guildVars.AdminRoleID, member) {
			c.session.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: 🚫 **Not authorized**", m.Author.ID))
			return
		}

		rest := content[len(adminCommandPrefix):]
		firstSpaceIndex := strings.Index(rest, " ")

		var commandName string
		if firstSpaceIndex == -1 {
			commandName = rest
			rest = ""
		} else {
			commandName = rest[:firstSpaceIndex]
			rest = strings.TrimSpace(rest[firstSpaceIndex+1:])
		}

		cmd, ok := adminCommands[commandName]
		if !ok {
			if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s` not found**", m.Author.ID, adminCommandPrefix, commandName))
			}
			return
		}

		if err := cmd(ctx, c, guildVars, m.Message, channel, rest); err != nil {
			glog.Errorf("Failed to run admin command %s: %v", commandName, err)
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

func showHelp(ctx context.Context, c *Client, guildVars *varstore.GuildVars, m *discordgo.Message, channel *discordgo.Channel) error {
	var aliases map[string]*varstore.Alias
	ok, err := func() (bool, error) {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()

		aliases, err = c.vars.GuildAliases(ctx, tx, channel.GuildID)
		if err != nil {
			return false, err
		}

		return true, nil
	}()

	if err != nil {
		return err
	}

	if !ok {
		return nil
	}

	// Find all distinct commands to request.
	aliasNames := make([]string, 0, len(aliases))
	for aliasName, _ := range aliases {
		aliasNames = append(aliasNames, aliasName)
	}

	aliasGroups := make(map[string][]int, 0)
	for i, aliasName := range aliasNames {
		alias := aliases[aliasName]

		qualifiedName := fmt.Sprintf("%s/%s", alias.OwnerName, alias.ScriptName)
		if aliasGroups[qualifiedName] == nil {
			aliasGroups[qualifiedName] = make([]int, 0)
		}
		aliasGroups[qualifiedName] = append(aliasGroups[qualifiedName], i)
	}

	uniqueAliases := make([]*varstore.Alias, 0, len(aliasGroups))
	for _, indexes := range aliasGroups {
		uniqueAliases = append(uniqueAliases, aliases[aliasNames[indexes[0]]])
	}

	uniqueAliasMetas := make([]*scriptspb.Meta, len(aliasGroups))

	g, ctx := errgroup.WithContext(ctx)

	for i, alias := range uniqueAliases {
		i := i
		alias := alias

		g.Go(func() error {
			getMeta, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
				OwnerName: alias.OwnerName,
				Name:      alias.ScriptName,
			})

			if err != nil {
				if grpc.Code(err) == codes.NotFound {
					return nil
				}
				return err
			}

			uniqueAliasMetas[i] = getMeta.Meta
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	fields := make([]*discordgo.MessageEmbedField, len(uniqueAliases))
	for i, alias := range uniqueAliases {
		qualifiedName := fmt.Sprintf("%s/%s", alias.OwnerName, alias.ScriptName)
		group := aliasGroups[qualifiedName]

		formattedNames := make([]string, len(group))
		for j, k := range group {
			formattedNames[j] = fmt.Sprintf("`%s%s`", guildVars.ScriptCommandPrefix, aliasNames[k])
		}

		meta := uniqueAliasMetas[i]
		description := "**Command not found. Contact an administrator.**"
		if meta != nil {
			description = strings.TrimSpace(meta.Description)
			if description == "" {
				description = "_No description set._"
			}
		}

		sort.Strings(formattedNames)

		fields[i] = &discordgo.MessageEmbedField{
			Name:  strings.Join(formattedNames, ", "),
			Value: description,
		}
	}

	sort.Sort(ByFieldName(fields))

	member, err := c.session.GuildMember(channel.GuildID, m.Author.ID)
	if err != nil {
		return err
	}

	if memberIsAdmin(guildVars.AdminRoleID, member) {
		fields = append(fields,
			&discordgo.MessageEmbedField{
				Name: fmt.Sprintf("`%saliasinfo <command name>`", adminCommandPrefix),
				Value: `**Administrators only.**
Get extended information on any command beginning with ` + "`" + guildVars.ScriptCommandPrefix + "`",
			},
			&discordgo.MessageEmbedField{
				Name: fmt.Sprintf("`%ssetalias <command name> <script name>`", adminCommandPrefix),
				Value: `**Administrators only.**
Alias a command name (short name) to a script name (long name). If the alias already exists, it will be replaced.`,
			},
			&discordgo.MessageEmbedField{
				Name: fmt.Sprintf("`%sdelalias <command name>`", adminCommandPrefix),
				Value: `**Administrators only.**
Remove a command name's alias.`,
			},
		)
	}

	formattedAnnouncement := ""
	if guildVars.Announcement != "" {
		formattedAnnouncement = fmt.Sprintf("\n\n📣 **%s**", guildVars.Announcement)
	}

	c.session.ChannelMessageSendComplex(m.ChannelID, &discordgo.MessageSend{
		Content: fmt.Sprintf("<@%s>: ✅", m.Author.ID),
		Embed: &discordgo.MessageEmbed{
			Title:       "ℹ Help",
			URL:         c.opts.WebURL,
			Description: fmt.Sprintf(`Here's a listing of my commands.%s`, formattedAnnouncement),
			Color:       0x009100,
			Fields:      fields,
		},
	})
	return nil
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

	if _, err := c.scriptsClient.GetMeta(ctx, &scriptspb.GetMetaRequest{
		OwnerName: ownerName,
		Name:      scriptName,
	}); err != nil {
		if grpc.Code(err) == codes.NotFound {
			if aliased {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❗ **Command alias references invalid script name**", m.Author.ID))
			} else if !guildVars.Quiet {
				s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("<@%s>: ❎ **Command `%s%s/%s` not found**", m.Author.ID, guildVars.ScriptCommandPrefix, ownerName, scriptName))
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
		},
		NetworkInfoServiceTarget: c.networkInfoServiceTarget,
	})
	if err != nil {
		return err
	}

	channelID := m.ChannelID
	if resp.Private {
		channel, err := s.UserChannelCreate(m.Author.ID)
		if err != nil {
			return err
		}

		channelID = channel.ID
	}

	waitStatus := syscall.WaitStatus(resp.WaitStatus)

	if waitStatus.ExitStatus() == 0 || waitStatus.ExitStatus() == 2 {
		outputFormatter, ok := outputFormatters[resp.OutputFormat]
		if !ok {
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s>: ❗ **Output format `%s` unknown!**", m.Author.ID, resp.OutputFormat))
			return nil
		}

		messageSend, err := outputFormatter(resp)
		if err != nil {
			if iErr, ok := err.(invalidOutputError); ok {
				s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
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

		if _, err := s.ChannelMessageSendComplex(channelID, messageSend); err != nil {
			return err
		}
		return nil
	} else if waitStatus.Signaled() {
		sig := waitStatus.Signal()
		switch sig {
		case syscall.SIGKILL:
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s>: ❗ **Script used too many resources!**", m.Author.ID))
		default:
			s.ChannelMessageSend(channelID, fmt.Sprintf("<@%s>: ❗ **Script was killed by %s!**", m.Author.ID, sig.String()))
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
			Content: fmt.Sprintf("<@%s>: ❗ **Error occurred!**", m.Author.ID),
			Embed:   &embed,
		})
	}

	return nil
}
