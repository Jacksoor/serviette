package contextservice

import (
	"encoding/json"
	"github.com/golang/protobuf/jsonpb"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	context *scriptspb.Context
}

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func New(context *scriptspb.Context) *Service {
	return &Service{
		context: context,
	}
}

func (s *Service) Context() *scriptspb.Context {
	return s.context
}

type GetRequest struct {
}

func (s *Service) Get(req *GetRequest, resp *map[string]interface{}) error {
	raw, err := marshaler.MarshalToString(s.context)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), resp)
}

type SetOutputFormatRequest struct {
	Format string `json:"format"`
}

type SetOutputFormatResponse struct{}

func (s *Service) SetOutputFormat(req *SetOutputFormatRequest, resp *SetOutputFormatResponse) error {
	s.context.OutputFormat = req.Format
	return nil
}
