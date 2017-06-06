package client

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
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

func resolveScriptName(ctx context.Context, c *Client, commandName string) ([]byte, string, bool, error) {
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
	resolveResp, err := c.scriptsClient.ResolveAlias(ctx, &scriptspb.ResolveAliasRequest{
		Name: commandName,
	})
	if err != nil {
		if grpc.Code(err) == codes.NotFound {
			return nil, "", true, errNotFound
		}
		return nil, "", true, err
	}

	return resolveResp.AccountHandle, resolveResp.ScriptName, true, nil

}
