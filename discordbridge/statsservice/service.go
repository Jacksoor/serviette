package statsservice

import (
	"golang.org/x/net/context"

	"github.com/porpoises/kobun4/discordbridge/statsstore"

	pb "github.com/porpoises/kobun4/executor/statsservice/v1pb"
)

type Service struct {
	stats *statsstore.Store
}

func New(stats *statsstore.Store) *Service {
	return &Service{
		stats: stats,
	}
}

func (s *Service) GetUserChannelStats(ctx context.Context, req *pb.GetUserChannelStatsRequest) (*pb.GetUserChannelStatsResponse, error) {
	stats, err := s.stats.UserChannelStats(ctx, req.UserId, req.ChannelId)
	if err != nil {
		return nil, err
	}

	return &pb.GetUserChannelStatsResponse{
		NumCharactersSent: stats.NumCharactersSent,
		NumMessagesSent:   stats.NumMessagesSent,
		LastResetTimeUnix: stats.LastResetTime.Unix(),
	}, nil
}
