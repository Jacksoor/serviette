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

	escrowedFunds int64

	charges map[string]int64
}

func New(moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, scriptAccountHandle []byte, executingAccountHandle []byte, escrowedFunds int64) *Service {
	return &Service{
		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		scriptAccountHandle:    scriptAccountHandle,
		executingAccountHandle: executingAccountHandle,

		escrowedFunds: escrowedFunds,

		charges: make(map[string]int64, 0),
	}
}

func (s *Service) Charges() map[string]int64 {
	return s.charges
}

func (s *Service) EscrowedFunds() int64 {
	return s.escrowedFunds
}

func (s *Service) GetEscrowedFunds(req *struct{}, resp *int64) error {
	*resp = s.escrowedFunds
	return nil
}

// Charge transfers money from the executing account's escrowed funds into a target account.
func (s *Service) Charge(req *struct {
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}, resp *struct{}) error {
	s.escrowedFunds -= req.Amount

	if s.escrowedFunds < 0 {
		return errors.New("charge would exceed the amount of escrowed funds")
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

	s.charges[string(targetAccountHandle)] += req.Amount

	return nil
}

// Pay transfers money from the script account into a target account.
func (s *Service) Pay(req *struct {
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}, resp *struct{}) error {
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

func (s *Service) Transfer(req *struct {
	SourceAccountHandle string `json:"sourceAccountHandle"`
	SourceAccountKey    string `json:"sourceAccountKey"`
	TargetAccountHandle string `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}, resp *struct{}) error {
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

	getResp, err := s.accountsClient.Get(context.Background(), &accountspb.GetRequest{
		AccountHandle: sourceAccountHandle,
	})

	if err != nil {
		return err
	}

	if string(getResp.AccountKey) != string(sourceAccountKey) {
		return errors.New("invalid account key")
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

func (s *Service) GetBalance(req *struct {
	AccountHandle string `json:"accountHandle"`
}, resp *int64) error {
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

	*resp = getBalanceResp.Balance
	return nil
}
