package outputservice

import (
	"fmt"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	traits       *accountspb.Traits
	outputParams *scriptspb.OutputParams
}

func isOutputFormatAllowed(traits *accountspb.Traits, format string) bool {
	for _, allowedFormat := range traits.AllowedOutputFormat {
		if allowedFormat == format {
			return true
		}
	}
	return false
}

func New(traits *accountspb.Traits, outputParams *scriptspb.OutputParams) *Service {
	return &Service{
		traits:       traits,
		outputParams: outputParams,
	}
}

func (s *Service) SetFormat(req *struct {
	Format string `json:"format"`
}, resp *srpc.Response) error {
	if !isOutputFormatAllowed(s.traits, req.Format) {
		return fmt.Errorf(`output format "%s" is not allowed`, req.Format)
	}
	s.outputParams.Format = req.Format
	return nil
}

func (s *Service) SetPrivate(req *struct {
	Private bool `json:"private"`
}, resp *srpc.Response) error {
	s.outputParams.Private = req.Private
	return nil
}

func (s *Service) SetExpires(req *struct {
	Expires bool `json:"expires"`
}, resp *srpc.Response) error {
	s.outputParams.Expires = req.Expires
	return nil
}
