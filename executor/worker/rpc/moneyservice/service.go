package moneyservice

import (
	"encoding/base64"
	"errors"

	"golang.org/x/net/context"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

type Service struct {
	moneyClient    moneypb.MoneyClient
	accountsClient accountspb.AccountsClient

	scriptAccountHandle    []byte
	executingAccountHandle []byte

	withdrawalLimit int64

	withdrawals map[string]int64
}

func New(moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, scriptAccountHandle []byte, executingAccountHandle []byte, withdrawalLimit int64) *Service {
	return &Service{
		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		scriptAccountHandle:    scriptAccountHandle,
		executingAccountHandle: executingAccountHandle,

		withdrawalLimit: withdrawalLimit,

		withdrawals: make(map[string]int64, 0),
	}
}

func (s *Service) Withdrawals() map[string]int64 {
	return s.withdrawals
}

func (s *Service) WithdrawalLimit() int64 {
	return s.withdrawalLimit
}

type ChargeRequest struct {
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type ChargeResponse struct{}

// Charge transfers money from the executing account into a target account.
func (s *Service) Charge(req *ChargeRequest, resp *ChargeResponse) error {
	s.withdrawalLimit -= req.Amount

	if s.withdrawalLimit < 0 {
		return errors.New("transfer would exceed withdrawal limit")
	}

	targetAccountHandle, err := base64.RawURLEncoding.DecodeString(req.TargetAccountHandle)
	if err != nil {
		return err
	}

	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.executingAccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	s.withdrawals[string(targetAccountHandle)] += req.Amount

	return nil
}

type PayRequest struct {
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type PayResponse struct{}

// Pay transfers money from the script account into a target account.
func (s *Service) Pay(req *PayRequest, resp *PayResponse) error {
	targetAccountHandle, err := base64.RawURLEncoding.DecodeString(req.TargetAccountHandle)
	if err != nil {
		return err
	}

	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.scriptAccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

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

	sourceAccountKey, err := base64.RawURLEncoding.DecodeString(req.SourceAccountKey)
	if err != nil {
		return err
	}

	targetAccountHandle, err := base64.RawURLEncoding.DecodeString(req.TargetAccountHandle)
	if err != nil {
		return err
	}

	if _, err := s.accountsClient.Check(context.Background(), &accountspb.CheckRequest{
		AccountHandle: sourceAccountHandle,
		AccountKey:    sourceAccountKey,
	}); err != nil {
		return err
	}

	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: sourceAccountHandle,
		TargetAccountHandle: targetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	return nil
}

type GetBalanceRequest struct {
	AccountHandle string `json:"accountHandle"`
}

type GetBalanceResponse struct {
	Balance int64 `json:"balance"`
}

func (s *Service) GetBalance(req *GetBalanceRequest, resp *GetBalanceResponse) error {
	accountHandle, err := base64.RawURLEncoding.DecodeString(req.AccountHandle)
	if err != nil {
		return err
	}

	getBalanceResp, err := s.moneyClient.GetBalance(context.Background(), &moneypb.GetBalanceRequest{
		AccountHandle: accountHandle,
	})
	if err != nil {
		return err
	}

	resp.Balance = getBalanceResp.Balance
	return nil
}
