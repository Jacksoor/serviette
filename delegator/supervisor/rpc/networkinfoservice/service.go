package networkinfoservice

import (
	"golang.org/x/net/context"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"

	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	ctx               context.Context
	context           *scriptspb.Context
	networkInfoClient networkinfopb.NetworkInfoClient
}

func New(ctx context.Context, context *scriptspb.Context, networkInfoClient networkinfopb.NetworkInfoClient) *Service {
	return &Service{
		ctx:               ctx,
		context:           context,
		networkInfoClient: networkInfoClient,
	}
}

func (s *Service) GetUserInfo(req *struct {
	ID string `json:"id"`
}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetUserInfo(s.ctx, &networkinfopb.GetUserInfoRequest{
		Context: s.context,
		UserId:  req.ID,
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
		Context:   s.context,
		ChannelId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Body = &struct {
		Name       string            `json:"name"`
		IsOneOnOne bool              `json:"isOneOnOne"`
		Extra      map[string]string `json:"extra"`
	}{
		grpcResp.Name,
		grpcResp.IsOneOnOne,
		grpcResp.Extra,
	}
	return nil
}

func (s *Service) GetGroupInfo(req *struct{}, resp *srpc.Response) error {
	grpcResp, err := s.networkInfoClient.GetGroupInfo(s.ctx, &networkinfopb.GetGroupInfoRequest{
		Context: s.context,
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
		Context:   s.context,
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
		Context: s.context,
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
