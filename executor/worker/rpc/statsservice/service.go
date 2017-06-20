package statsservice

import (
	"golang.org/x/net/context"

	statspb "github.com/porpoises/kobun4/executor/statsservice/v1pb"
)

type Service struct {
	ctx         context.Context
	statsClient statspb.StatsClient
}

func New(ctx context.Context, statsClient statspb.StatsClient) *Service {
	return &Service{
		ctx:         ctx,
		statsClient: statsClient,
	}
}

func (s *Service) GetUserChannelStats(req *struct {
	UserID    string `json:"userID"`
	ChannelID string `json:"channelID"`
}, resp *struct {
	NumCharactersSent int64 `json:"numCharactersSent"`
	NumMessagesSent   int64 `json:"numMessagesSent"`
	LastResetTimeUnix int64 `json:"lastResetTimeUnix"`
}) error {
	grpcResp, err := s.statsClient.GetUserChannelStats(s.ctx, &statspb.GetUserChannelStatsRequest{
		UserId:    req.UserID,
		ChannelId: req.ChannelID,
	})
	if err != nil {
		return err
	}

	resp.NumCharactersSent = grpcResp.NumCharactersSent
	resp.NumMessagesSent = grpcResp.NumMessagesSent
	resp.LastResetTimeUnix = grpcResp.LastResetTimeUnix
	return nil
}
