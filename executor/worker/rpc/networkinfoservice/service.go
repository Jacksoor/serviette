package networkinfoservice

import (
	"encoding/base64"

	"golang.org/x/net/context"

	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
)

type Service struct {
	networkInfoClient networkinfopb.NetworkInfoClient
}

func New(networkInfoClient networkinfopb.NetworkInfoClient) *Service {
	return &Service{
		networkInfoClient: networkInfoClient,
	}
}

func (s *Service) GetUserInfo(req *struct {
	ID string `json:"id"`
}, resp *struct {
	Name          string            `json:"name"`
	AccountHandle string            `json:"accountHandle"`
	Extra         map[string]string `json:"extra"`
}) error {
	grpcResp, err := s.networkInfoClient.GetUserInfo(context.Background(), &networkinfopb.GetUserInfoRequest{
		UserId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Name = grpcResp.Name
	resp.AccountHandle = base64.RawURLEncoding.EncodeToString(grpcResp.AccountHandle)
	resp.Extra = grpcResp.Extra
	return nil
}

func (s *Service) GetChannelInfo(req *struct {
	ID string `json:"id"`
}, resp *struct {
	Name       string            `json:"name"`
	IsOneOnOne bool              `json:"isOneOnOne"`
	Extra      map[string]string `json:"extra"`
}) error {
	grpcResp, err := s.networkInfoClient.GetChannelInfo(context.Background(), &networkinfopb.GetChannelInfoRequest{
		ChannelId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Name = grpcResp.Name
	resp.IsOneOnOne = grpcResp.IsOneOnOne
	resp.Extra = grpcResp.Extra
	return nil
}

func (s *Service) GetGroupInfo(req *struct {
	ID string `json:"id"`
}, resp *struct {
	Name  string            `json:"name"`
	Extra map[string]string `json:"extra"`
}) error {
	grpcResp, err := s.networkInfoClient.GetGroupInfo(context.Background(), &networkinfopb.GetGroupInfoRequest{
		GroupId: req.ID,
	})
	if err != nil {
		return err
	}

	resp.Name = grpcResp.Name
	resp.Extra = grpcResp.Extra
	return nil
}

func (s *Service) GetChannelMemberInfo(req *struct {
	ChannelID string `json:"channelId"`
	UserID    string `json:"userId"`
}, resp *struct {
	Name  string            `json:"name"`
	Extra map[string]string `json:"extra"`
}) error {
	grpcResp, err := s.networkInfoClient.GetChannelMemberInfo(context.Background(), &networkinfopb.GetChannelMemberInfoRequest{
		ChannelId: req.ChannelID,
		UserId:    req.UserID,
	})
	if err != nil {
		return err
	}

	resp.Name = grpcResp.Name
	resp.Extra = grpcResp.Extra
	return nil
}

func (s *Service) GetGroupMemberInfo(req *struct {
	GroupID string `json:"groupId"`
	UserID  string `json:"userId"`
}, resp *struct {
	Name  string            `json:"name"`
	Extra map[string]string `json:"extra"`
}) error {
	grpcResp, err := s.networkInfoClient.GetGroupMemberInfo(context.Background(), &networkinfopb.GetGroupMemberInfoRequest{
		GroupId: req.GroupID,
		UserId:  req.UserID,
	})
	if err != nil {
		return err
	}

	resp.Name = grpcResp.Name
	resp.Extra = grpcResp.Extra
	return nil
}
