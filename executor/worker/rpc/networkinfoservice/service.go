package networkinfoservice

import (
	"encoding/json"
	"github.com/golang/protobuf/jsonpb"

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

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func (s *Service) GetUserInfo(req *struct {
	ID string `json:"id"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.networkInfoClient.GetUserInfo(context.Background(), &networkinfopb.GetUserInfoRequest{
		UserId: req.ID,
	})
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetChannelInfo(req *struct {
	ID string `json:"id"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.networkInfoClient.GetChannelInfo(context.Background(), &networkinfopb.GetChannelInfoRequest{
		ChannelId: req.ID,
	})
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetGroupInfo(req *struct {
	ID string `json:"id"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.networkInfoClient.GetGroupInfo(context.Background(), &networkinfopb.GetGroupInfoRequest{
		GroupId: req.ID,
	})
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetChannelMemberInfo(req *struct {
	ChannelID string `json:"channelId"`
	UserID    string `json:"userId"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.networkInfoClient.GetChannelMemberInfo(context.Background(), &networkinfopb.GetChannelMemberInfoRequest{
		ChannelId: req.ChannelID,
		UserId:    req.UserID,
	})
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetGroupMemberInfo(req *struct {
	GroupID string `json:"groupId"`
	UserID  string `json:"userId"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.networkInfoClient.GetGroupMemberInfo(context.Background(), &networkinfopb.GetGroupMemberInfoRequest{
		GroupId: req.GroupID,
		UserId:  req.UserID,
	})
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}
