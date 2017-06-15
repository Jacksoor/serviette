package messagingservice

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/client"
	"github.com/porpoises/kobun4/discordbridge/varstore"

	pb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
)

type Service struct {
	session *discordgo.Session
	vars    *varstore.Store
}

func New(session *discordgo.Session, vars *varstore.Store) *Service {
	return &Service{
		session: session,
		vars:    vars,
	}
}

func (s *Service) Message(ctx context.Context, req *pb.MessageRequest) (*pb.MessageResponse, error) {
	var channelID string
	switch t := req.Target.(type) {
	case *pb.MessageRequest_ChannelId:
		channelID = t.ChannelId
	case *pb.MessageRequest_UserId:
		channel, err := s.session.UserChannelCreate(t.UserId)
		if err != nil {
			return nil, err
		}
		channelID = channel.ID
	}

	outputFormatter, ok := client.OutputFormatters[req.Format]
	if !ok {
		return nil, fmt.Errorf("formatter %s not found", req.Format)
	}

	messageSend, err := outputFormatter("", req.Content, true)
	if err != nil {
		return nil, err
	}

	if _, err := s.session.ChannelMessageSendComplex(channelID, messageSend); err != nil {
		return nil, err
	}

	return &pb.MessageResponse{}, nil
}
