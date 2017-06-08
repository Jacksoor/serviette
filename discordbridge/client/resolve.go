package client

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/net/context"

	"github.com/porpoises/kobun4/discordbridge/varstore"
)

var (
	errNotFound         error = errors.New("not found")
	errBadAccountHandle       = errors.New("bad account handle")
)

func resolveAccountTarget(ctx context.Context, tx *sql.Tx, c *Client, target string) ([]byte, error) {
	matches := discordMentionRegexp.FindStringSubmatch(target)
	if len(matches) > 0 && matches[0] == target {
		userVars, err := c.vars.UserVars(ctx, tx, matches[1])
		if err != nil {
			if err == varstore.ErrNotFound {
				return nil, errNotFound
			}
			return nil, err
		}

		return userVars.AccountHandle, nil
	}

	accountHandle, err := base64.RawURLEncoding.DecodeString(target)
	if err != nil {
		return nil, errBadAccountHandle
	}

	return accountHandle, nil
}

func resolveScriptName(ctx context.Context, c *Client, guildID string, commandName string) ([]byte, string, bool, error) {
	if sepIndex := strings.Index(commandName, "/"); sepIndex != -1 {
		// Look up via qualified name.
		encodedScriptHandle := commandName[:sepIndex]
		scriptHandle, err := base64.RawURLEncoding.DecodeString(encodedScriptHandle)
		if err != nil {
			return nil, "", false, errNotFound
		}
		name := commandName[sepIndex+1:]
		return scriptHandle, name, false, nil
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
			return nil, "", true, errNotFound
		}
		return nil, "", true, err
	}

	return alias.AccountHandle, alias.ScriptName, true, nil
}
