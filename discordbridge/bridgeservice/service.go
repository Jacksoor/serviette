package bridgeservice

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	pb "github.com/porpoises/kobun4/executor/bridgeservice/v1pb"
)

type Service struct {
	session *discordgo.Session
}

func New(session *discordgo.Session) *Service {
	return &Service{
		session: session,
	}
}

func (s *Service) GetUserInfo(ctx context.Context, req *pb.GetUserInfoRequest) (*pb.GetUserInfoResponse, error) {
	user, err := s.session.User(req.UserId)
	if err != nil {
		return nil, err
	}

	return &pb.GetUserInfoResponse{
		Name: fmt.Sprintf("%s#%s", user.Username, user.Discriminator),
	}, nil
}

func (s *Service) GetChannelInfo(ctx context.Context, req *pb.GetChannelInfoRequest) (*pb.GetChannelInfoResponse, error) {
	channel, err := s.session.Channel(req.ChannelId)
	if err != nil {
		return nil, err
	}

	return &pb.GetChannelInfoResponse{
		Name:       channel.Name,
		IsOneOnOne: channel.IsPrivate,
	}, nil
}

func (s *Service) GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error) {
	guild, err := s.session.Guild(req.GroupId)
	if err != nil {
		return nil, err
	}

	return &pb.GetGroupInfoResponse{
		Name: guild.Name,
	}, nil
}

func (s *Service) GetNetworkInfo(ctx context.Context, req *pb.GetNetworkInfoRequest) (*pb.GetNetworkInfoResponse, error) {
	if req.NetworkId != "discord" {
		return nil, fmt.Errorf(`network_id can only be "discord"`)
	}

	return &pb.GetNetworkInfoResponse{
		Name: "Discord",
	}, nil
}
