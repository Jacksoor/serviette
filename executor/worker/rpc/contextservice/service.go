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

type ContextRequest struct {
}

func (s *Service) Get(req *ContextRequest, resp *scriptspb.Context) error {
	*resp = *s.context
	return nil
}
