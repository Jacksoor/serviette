package moneyservice

import (
	"encoding/base64"

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

type PayRequest struct {
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type PayResponse struct{}

// Pay transfers money from the executing account into a target account.
func (s *Service) Pay(req *PayRequest, resp *PayResponse) error {
	targetAccountHandle, err := base64.RawURLEncoding.DecodeString(req.TargetAccountHandle)
	if err != nil {
		return err
	}

	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.accountHandle,
		SourceAccountKey:    s.accountKey,
		TargetAccountHandle: targetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	s.withdrawals[string(targetAccountHandle)] += req.Amount

	return nil
}

type TransferRequest struct {
	SourceAccountHandle string `json:"sourceAccountHandle"`
	SourceAccountKey    string `json:"sourceAccountKey"`
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type TransferResponse struct{}

func (s *Service) Transfer(req *TransferRequest, resp *TransferResponse) error {
	sourceAccountHandle, err := base64.RawURLEncoding.DecodeString(req.SourceAccountHandle)
	if err != nil {
		return err
	}

	targetAccountHandle, err := base64.RawURLEncoding.DecodeString(req.TargetAccountHandle)
	if err != nil {
		return err
	}

	sourceAccountKey, err := base64.RawURLEncoding.DecodeString(req.SourceAccountKey)
	if err != nil {
		return err
	}

	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: sourceAccountHandle,
		SourceAccountKey:    sourceAccountKey,
		TargetAccountHandle: targetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	if string(req.SourceAccountHandle) == string(s.accountHandle) {
		s.withdrawals[string(req.TargetAccountHandle)] += req.Amount
	}

	return nil
}
