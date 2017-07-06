package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"
)

type statsReporter func(ctx context.Context, userID string, shardID int, shardCount int, serverCount int) error

type statsAuthTokenContextKey string

func postStatsSimple(ctx context.Context, provider string, userID string, shardID int, shardCount int, serverCount int) error {
	raw, err := json.Marshal(struct {
		ShardID     int `json:"shard_id"`
		ShardCount  int `json:"shard_count"`
		ServerCount int `json:"server_count"`
	}{
		shardID,
		shardCount,
		serverCount,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://%s/api/bots/%s/stats", provider, userID), bytes.NewBuffer(raw))
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", ctx.Value(statsAuthTokenContextKey(provider)).(string))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to update bot stats: %d %s", resp.StatusCode, resp.Status)
	}

	return nil
}

var statsReporters map[string]statsReporter = map[string]statsReporter{
	"bots.discord.pw": func(ctx context.Context, userID string, shardID int, shardCount int, serverCount int) error {
		return postStatsSimple(ctx, "bots.discord.pw", userID, shardID, shardCount, serverCount)
	},
	"discordbots.org": func(ctx context.Context, userID string, shardID int, shardCount int, serverCount int) error {
		return postStatsSimple(ctx, "discordbots.org", userID, shardID, shardCount, serverCount)
	},
}
