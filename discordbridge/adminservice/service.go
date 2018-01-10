package adminservice

import (
	"errors"

	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	pb "github.com/porpoises/kobun4/executor/adminservice/v1pb"
)

type Service struct {
	session *discordgo.Session
}

func New(session *discordgo.Session) *Service {
	return &Service{
		session: session,
	}
}

func (s *Service) DeleteMessage(ctx context.Context, req *pb.DeleteMessageRequest) (*pb.DeleteMessageResponse, error) {
	channel, err := s.session.State.Channel(req.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.Type == discordgo.ChannelTypeGuildText && channel.GuildID != req.Context.GroupId {
		return nil, errors.New("not permitted to delete this message")
	}

	if err := s.session.ChannelMessageDelete(req.ChannelId, req.MessageId); err != nil {
		return nil, err
	}

	return &pb.DeleteMessageResponse{}, nil
}
