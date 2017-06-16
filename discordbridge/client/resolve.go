package client

import (
	"errors"
	"strings"

	"golang.org/x/net/context"

	"github.com/porpoises/kobun4/discordbridge/varstore"
)

var (
	errNotFound error = errors.New("not found")
)

func commandNameIsLinked(commandName string) bool {
	return !strings.Contains(commandName, "/")
}

func resolveScriptName(ctx context.Context, c *Client, guildID string, commandName string) (string, string, error) {
	if sepIndex := strings.Index(commandName, "/"); sepIndex != -1 {
		// Look up via qualified name.
		ownerName := commandName[:sepIndex]
		name := commandName[sepIndex+1:]
		return ownerName, name, nil
	}

	// Look up via an link name.
	var link *varstore.Link
	if err := func() error {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		link, err = c.vars.GuildLink(ctx, tx, guildID, commandName)
		return err
	}(); err != nil {
		if err == varstore.ErrNotFound {
			return "", "", errNotFound
		}
		return "", "", err
	}

	return link.OwnerName, link.ScriptName, nil
}
