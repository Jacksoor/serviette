package networkinfoservice

import (
	"golang.org/x/net/context"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"

	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
)

type Service struct {
	ctx               context.Context
	networkInfoClient networkinfopb.NetworkInfoClient
}

func New(ctx context.Context, networkInfoClient networkinfopb.NetworkInfoClient) *Service {
	return &Service{
		ctx:               ctx,
		networkInfoClient: networkInfoClient,
	}
}

func (s *Service) GetUserInfo(req *struct {
	ID string `json:"id"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetUserInfo(s.ctx, &networkinfopb.GetUserInfoRequest{
		UserId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name  string            `json:"name"`
		Extra map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.Extra,
	}
	return nil
}

func (s *Service) GetChannelInfo(req *struct {
	ID string `json:"id"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetChannelInfo(s.ctx, &networkinfopb.GetChannelInfoRequest{
		ChannelId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name       string            `json:"name"`
		IsOneOnOne bool              `json:"is_one_on_one"`
		Extra      map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.IsOneOnOne,
		grpcResp.Extra,
	}
	return nil
}

func (s *Service) GetGroupInfo(req *struct {
	ID string `json:"id"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetGroupInfo(s.ctx, &networkinfopb.GetGroupInfoRequest{
		GroupId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name  string            `json:"name"`
		Extra map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.Extra,
	}
	return nil
}

func (s *Service) GetChannelMemberInfo(req *struct {
	ChannelID string `json:"channelId"`
	UserID    string `json:"userId"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetChannelMemberInfo(s.ctx, &networkinfopb.GetChannelMemberInfoRequest{
		ChannelId: req.ChannelID,
		UserId:    req.UserID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name  string            `json:"name"`
		Roles []string          `json:"roles"`
		Extra map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.Role,
		grpcResp.Extra,
	}
	return nil
}

func (s *Service) GetGroupMemberInfo(req *struct {
	GroupID string `json:"groupId"`
	UserID  string `json:"userId"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetGroupMemberInfo(s.ctx, &networkinfopb.GetGroupMemberInfoRequest{
		GroupId: req.GroupID,
		UserId:  req.UserID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name  string            `json:"name"`
		Roles []string          `json:"roles"`
		Extra map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.Role,
		grpcResp.Extra,
	}
	return nil
}
