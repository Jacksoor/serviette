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

func resolveScriptName(ctx context.Context, c *Client, guildID string, commandName string) (string, string, bool, error) {
	if sepIndex := strings.Index(commandName, "/"); sepIndex != -1 {
		// Look up via qualified name.
		ownerName := commandName[:sepIndex]
		name := commandName[sepIndex+1:]
		return ownerName, name, false, nil
	}

	// Look up via an alias name.
	var alias *varstore.Alias
	if err := func() error {
		tx, err := c.vars.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()

		alias, err = c.vars.GuildAlias(ctx, tx, guildID, commandName)
		return err
	}(); err != nil {
		if err == varstore.ErrNotFound {
			return "", "", true, errNotFound
		}
		return "", "", true, err
	}

	return alias.OwnerName, alias.ScriptName, true, nil
}
