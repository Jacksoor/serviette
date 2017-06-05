package networkinfoservice

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	pb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
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
		Name:       fmt.Sprintf("#%s", channel.Name),
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

func (s *Service) GetGroupMemberInfo(ctx context.Context, req *pb.GetGroupMemberInfoRequest) (*pb.GetGroupMemberInfoResponse, error) {
	user, err := s.session.User(req.UserId)
	if err != nil {
		return nil, err
	}

	name := user.Username

	member, err := s.session.GuildMember(req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}

	if member.Nick != "" {
		name = member.Nick
	}

	return &pb.GetGroupMemberInfoResponse{
		Name: name,
	}, nil
}

func (s *Service) GetChannelMemberInfo(ctx context.Context, req *pb.GetChannelMemberInfoRequest) (*pb.GetChannelMemberInfoResponse, error) {
	user, err := s.session.User(req.UserId)
	if err != nil {
		return nil, err
	}

	name := user.Username

	channel, err := s.session.Channel(req.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.GuildID != "" {
		member, err := s.session.GuildMember(channel.GuildID, req.UserId)
		if err != nil {
			return nil, err
		}

		if member.Nick != "" {
			name = member.Nick
		}
	}

	return &pb.GetChannelMemberInfoResponse{
		Name: name,
	}, nil
}
