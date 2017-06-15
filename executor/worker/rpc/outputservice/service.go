package outputservice

import (
	"errors"

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
	if req.Format == "raw" && !s.account.AllowRawOutput {
		return errors.New("raw format requested but account is not allowed to send raw output")
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
