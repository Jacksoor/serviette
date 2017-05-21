package moneyservice

import (
	"golang.org/x/net/context"

	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

type Service struct {
	moneyClient moneypb.MoneyClient

	accountHandle []byte
	accountKey    []byte

	withdrawals map[string]int64
}

func New(moneyClient moneypb.MoneyClient, accountHandle []byte, accountKey []byte) *Service {
	return &Service{
		moneyClient: moneyClient,

		accountHandle: accountHandle,
		accountKey:    accountKey,

		withdrawals: make(map[string]int64, 0),
	}
}

func (s *Service) Withdrawals() map[string]int64 {
	return s.withdrawals
}

type ChargeRequest struct {
	TargetAccountHandle []byte `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type ChargeResponse struct{}

// Charge transfers money from the executing account into a target account.
func (s *Service) Charge(req *ChargeRequest, resp *ChargeResponse) error {
	if err := s.Transfer(&TransferRequest{
		SourceAccountHandle: s.accountHandle,
		SourceAccountKey:    s.accountKey,
		TargetAccountHandle: req.TargetAccountHandle,
		Amount:              req.Amount,
	}, nil); err != nil {
		return err
	}

	return nil
}

type TransferRequest struct {
	SourceAccountHandle []byte `json:"sourceAccountHandle"`
	SourceAccountKey    []byte `json:"sourceAccountKey"`
	TargetAccountHandle []byte `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type TransferResponse struct{}

func (s *Service) Transfer(req *TransferRequest, resp *TransferResponse) error {
	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: req.SourceAccountHandle,
		SourceAccountKey:    req.SourceAccountKey,
		TargetAccountHandle: req.TargetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	if string(req.SourceAccountHandle) == string(s.accountHandle) {
		s.withdrawals[string(req.TargetAccountHandle)] += req.Amount
	}

	return nil
}
