package client

import (
	"github.com/bwmarrin/discordgo"

	"golang.org/x/net/context"
)

type command func(ctx context.Context, c *Client, s *discordgo.Session, m *discordgo.Message, rest string) error
