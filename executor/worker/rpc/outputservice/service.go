package outputservice

import (
	"fmt"

	"github.com/porpoises/kobun4/executor/accounts"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	account      *accounts.Account
	OutputParams *scriptspb.OutputParams
}

func New(account *accounts.Account) *Service {
	return &Service{
		account: account,
		OutputParams: &scriptspb.OutputParams{
			Format: "text",
		},
	}
}

func (s *Service) SetFormat(req *struct {
	Format string `json:"format"`
}, resp *struct{}) error {
	if !s.account.IsOutputFormatAllowed(req.Format) {
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
