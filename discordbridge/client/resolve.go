package client

import (
	"encoding/base64"
	"errors"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
)

var (
	errNotFound         error = errors.New("not found")
	errBadAccountHandle       = errors.New("bad account handle")
)

func resolveAccountTarget(ctx context.Context, c *Client, target string) ([]byte, error) {
	matches := discordMentionRegexp.FindStringSubmatch(target)
	if len(matches) > 0 && matches[0] == target {
		resolveResp, err := c.accountsClient.ResolveAlias(ctx, &accountspb.ResolveAliasRequest{
			Name: aliasName(matches[1]),
		})
		if err != nil {
			if grpc.Code(err) == codes.NotFound {
				return nil, errNotFound
			}
			return nil, err
		}

		return resolveResp.AccountHandle, nil
	}

	accountHandle, err := base64.RawURLEncoding.DecodeString(target)
	if err != nil {
		return nil, errBadAccountHandle
	}

	return accountHandle, nil
}

func resolveScriptName(ctx context.Context, c *Client, commandName string) ([]byte, string, error) {
	sepIndex := strings.Index(commandName, ":")
	if sepIndex != -1 {
		// Look up via qualified name.
		encodedScriptHandle := commandName[:sepIndex]
		scriptHandle, err := base64.RawURLEncoding.DecodeString(encodedScriptHandle)
		if err != nil {
			return nil, "", errNotFound
		}
		name := commandName[sepIndex+1:]
		return scriptHandle, name, nil
	} else {
		// Look up via an alias name.
		contentResp, err := c.deedsClient.GetContent(ctx, &deedspb.GetContentRequest{
			Type: "command",
			Name: commandName,
		})
		if err != nil {
			if grpc.Code(err) == codes.NotFound {
				return nil, "", errNotFound
			}
			return nil, "", err
		}
		return nil, string(contentResp.Content), errNotFound
	}
	return nil, "", errNotFound
}
