package moneyservice

import (
	"golang.org/x/net/context"

	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

type Service struct {
	moneyClient moneypb.MoneyClient

	accountHandle []byte
	accountKey    []byte

	transfers int64
}

func New(moneyClient moneypb.MoneyClient, accountHandle []byte, accountKey []byte) *Service {
	return &Service{
		moneyClient: moneyClient,

		accountHandle: accountHandle,
		accountKey:    accountKey,

		transfers: 0,
	}
}

type ChargeRequest struct {
	TargetAccountHandle []byte `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type ChargeResponse struct{}

// Charge transfers money from the executing account into a target account.
func (s *Service) Charge(req *ChargeRequest, resp *ChargeResponse) error {
	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.accountHandle,
		SourceAccountKey:    s.accountKey,
		TargetAccountHandle: req.TargetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	s.transfers += req.Amount

	resp = &ChargeResponse{}
	return nil
}

func (s *Service) Transfers() int64 {
	return s.transfers
}
