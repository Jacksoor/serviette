package statsservice

import (
	"golang.org/x/net/context"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"

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
}, resp *srpc.Response) error {
	grpcResp, err := s.statsClient.GetUserChannelStats(s.ctx, &statspb.GetUserChannelStatsRequest{
		UserId:    req.UserID,
		ChannelId: req.ChannelID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		NumCharactersSent int64 `json:"numCharactersSent"`
		NumMessagesSent   int64 `json:"numMessagesSent"`
		LastResetTimeUnix int64 `json:"lastResetTimeUnix"`
	}{
		grpcResp.NumCharactersSent,
		grpcResp.NumMessagesSent,
		grpcResp.LastResetTimeUnix,
	}
	return nil
}
