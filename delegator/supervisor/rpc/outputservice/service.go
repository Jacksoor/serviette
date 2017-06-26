package outputservice

import (
	"fmt"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	traits       *accountspb.Traits
	OutputParams *scriptspb.OutputParams
}

func isOutputFormatAllowed(traits *accountspb.Traits, format string) bool {
	for _, allowedFormat := range traits.AllowedOutputFormat {
		if allowedFormat == format {
			return true
		}
	}
	return false
}

func New(traits *accountspb.Traits) *Service {
	return &Service{
		traits: traits,
		OutputParams: &scriptspb.OutputParams{
			Format: "text",
		},
	}
}

func (s *Service) SetFormat(req *struct {
	Format string `json:"format"`
}, resp *struct{}) error {
	if !isOutputFormatAllowed(s.traits, req.Format) {
		return fmt.Errorf(`output format "%s" is not allowed`, req.Format)
	}
	s.OutputParams.Format = req.Format
	return nil
}

func (s *Service) SetPrivate(req *struct {
	Private bool `json:"private"`
}, resp *struct{}) error {
	s.OutputParams.Private = req.Private
	return nil
}
