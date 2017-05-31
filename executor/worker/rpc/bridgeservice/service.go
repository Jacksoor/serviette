package bridgeservice

import (
	"encoding/json"
	"github.com/golang/protobuf/jsonpb"

	"golang.org/x/net/context"

	bridgepb "github.com/porpoises/kobun4/executor/bridgeservice/v1pb"
)

type Service struct {
	bridgeClient bridgepb.BridgeClient
}

func New(bridgeClient bridgepb.BridgeClient) *Service {
	return &Service{
		bridgeClient: bridgeClient,
	}
}

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func (s *Service) GetUserInfo(req *struct {
	ID string `json:"id"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.bridgeClient.GetUserInfo(context.Background(), &bridgepb.GetUserInfoRequest{
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
	grpcResp, err := s.bridgeClient.GetChannelInfo(context.Background(), &bridgepb.GetChannelInfoRequest{
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

func (s *Service) GetServerInfo(req *struct {
	ID string `json:"id"`
}, resp *map[string]interface{}) error {
	grpcResp, err := s.bridgeClient.GetServerInfo(context.Background(), &bridgepb.GetServerInfoRequest{
		ServerId: req.ID,
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
