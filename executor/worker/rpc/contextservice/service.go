package contextservice

import (
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	context *scriptspb.Context
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

func (s *Service) Get(req *GetRequest, resp *scriptspb.Context) error {
	*resp = *s.context
	return nil
}

type SetOutputFormatRequest struct {
	Format string `json:"format"`
}

type SetOutputFormatResponse struct{}

func (s *Service) SetOutputFormat(req *SetOutputFormatRequest, resp *SetOutputFormatResponse) error {
	s.context.OutputFormat = req.Format
	return nil
}
