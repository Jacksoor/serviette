package bridgeservice

import (
	"encoding/json"
	"github.com/golang/protobuf/jsonpb"

	"golang.org/x/net/context"

	bridgepb "github.com/porpoises/kobun4/executor/bridgeservice/v1pb"
)

type Service struct {
	name         string
	bridgeClient bridgepb.BridgeClient
}

func New(name string, bridgeClient bridgepb.BridgeClient) *Service {
	return &Service{
		name:         name,
		bridgeClient: bridgeClient,
	}
}

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func (s *Service) GetUserInfo(req *map[string]interface{}, resp *map[string]interface{}) error {
	grpcReq := &bridgepb.GetUserInfoRequest{}

	rawReq, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if err := jsonpb.UnmarshalString(string(rawReq), grpcReq); err != nil {
		return err
	}

	grpcResp, err := s.bridgeClient.GetUserInfo(context.Background(), grpcReq)
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetChannelInfo(req *map[string]interface{}, resp *map[string]interface{}) error {
	grpcReq := &bridgepb.GetChannelInfoRequest{}

	rawReq, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if err := jsonpb.UnmarshalString(string(rawReq), grpcReq); err != nil {
		return err
	}

	grpcResp, err := s.bridgeClient.GetChannelInfo(context.Background(), grpcReq)
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}

func (s *Service) GetServerInfo(req *map[string]interface{}, resp *map[string]interface{}) error {
	grpcReq := &bridgepb.GetServerInfoRequest{}

	rawReq, err := json.Marshal(req)
	if err != nil {
		return err
	}

	if err := jsonpb.UnmarshalString(string(rawReq), grpcReq); err != nil {
		return err
	}

	grpcResp, err := s.bridgeClient.GetServerInfo(context.Background(), grpcReq)
	if err != nil {
		return err
	}

	rawResp, err := marshaler.MarshalToString(grpcResp)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(rawResp), resp)
}
