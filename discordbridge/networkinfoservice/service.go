package networkinfoservice

import (
	"errors"
	"fmt"

	"golang.org/x/net/context"

	"github.com/bwmarrin/discordgo"

	"github.com/porpoises/kobun4/discordbridge/varstore"

	pb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
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

func (s *Service) GetUserInfo(ctx context.Context, req *pb.GetUserInfoRequest) (*pb.GetUserInfoResponse, error) {
	member, err := s.session.State.Member(req.Context.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}

	user := member.User

	return &pb.GetUserInfoResponse{
		Name: fmt.Sprintf("%s#%s", user.Username, user.Discriminator),
	}, nil
}

func (s *Service) GetChannelInfo(ctx context.Context, req *pb.GetChannelInfoRequest) (*pb.GetChannelInfoResponse, error) {
	// Short-circuit this if we're getting the current channel info.
	if req.ChannelId != req.Context.ChannelId {
		guild, err := s.session.State.Guild(req.Context.GroupId)
		if err != nil {
			return nil, err
		}

		found := false
		for _, channel := range guild.Channels {
			if channel.ID == req.ChannelId {
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("channel not found in guild")
		}
	}

	channel, err := s.session.State.Channel(req.ChannelId)
	if err != nil {
		return nil, err
	}

	var name string
	if channel.Name != "" {
		name = fmt.Sprintf("#%s", channel.Name)
	}

	return &pb.GetChannelInfoResponse{
		Name:       name,
		IsOneOnOne: channel.IsPrivate,
	}, nil
}

func (s *Service) GetGroupInfo(ctx context.Context, req *pb.GetGroupInfoRequest) (*pb.GetGroupInfoResponse, error) {
	guild, err := s.session.State.Guild(req.Context.GroupId)
	if err != nil {
		return nil, err
	}

	return &pb.GetGroupInfoResponse{
		Name: guild.Name,
	}, nil
}

func (s *Service) GetGroupMemberInfo(ctx context.Context, req *pb.GetGroupMemberInfoRequest) (*pb.GetGroupMemberInfoResponse, error) {
	member, err := s.session.State.Member(req.Context.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}

	name := member.Nick
	if member.Nick != "" {
		name = member.User.Username
	}

	return &pb.GetGroupMemberInfoResponse{
		Name: name,
		Role: member.Roles,
	}, nil
}

func (s *Service) GetChannelMemberInfo(ctx context.Context, req *pb.GetChannelMemberInfoRequest) (*pb.GetChannelMemberInfoResponse, error) {
	// Short-circuit this if we're getting the current channel info.
	if req.ChannelId != req.Context.ChannelId {
		guild, err := s.session.State.Guild(req.Context.GroupId)
		if err != nil {
			return nil, err
		}

		found := false
		for _, channel := range guild.Channels {
			if channel.ID == req.ChannelId {
				found = true
				break
			}
		}

		if !found {
			return nil, errors.New("channel not found in guild")
		}
	}

	channel, err := s.session.State.Channel(req.ChannelId)
	if err != nil {
		return nil, err
	}

	member, err := s.session.State.Member(req.Context.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}

	user := member.User
	name := user.Username

	var roles []string
	if channel.GuildID != "" {
		member, err := s.session.State.Member(channel.GuildID, req.UserId)
		if err != nil {
			return nil, err
		}

		if member.Nick != "" {
			name = member.Nick
		}

		roles = member.Roles
	}

	return &pb.GetChannelMemberInfoResponse{
		Name: name,
		Role: roles,
	}, nil
}
