package moneyservice

import (
	"golang.org/x/net/context"

	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

type Service struct {
	moneyClient moneypb.MoneyClient

	executingAccountHandle []byte
	executingAccountKey    []byte

	billingAccountHandle []byte
	billingAccountKey    []byte

	executingAccountTransfers int64
	billingAccountTransfers   int64
}

func New(moneyClient moneypb.MoneyClient, executingAccountHandle []byte, executingAccountKey []byte, billingAccountHandle []byte, billingAccountKey []byte) *Service {
	return &Service{
		moneyClient: moneyClient,

		executingAccountHandle: executingAccountHandle,
		executingAccountKey:    executingAccountKey,

		billingAccountHandle: billingAccountHandle,
		billingAccountKey:    billingAccountKey,

		executingAccountTransfers: 0,
		billingAccountTransfers:   0,
	}
}

type Transfers struct {
	ExecutingAccount int64
	BillingAccount   int64
}

type ChargeRequest struct {
	TargetAccountHandle []byte `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type ChargeResponse struct{}

// Charge transfers money from the executing account into a target account.
func (s *Service) Charge(req *ChargeRequest, resp *ChargeResponse) error {
	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.executingAccountHandle,
		SourceAccountKey:    s.executingAccountKey,
		TargetAccountHandle: req.TargetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	s.executingAccountTransfers += req.Amount

	resp = &ChargeResponse{}
	return nil
}

type BillRequest struct {
	TargetAccountHandle []byte `json:"targetAccountHandle"`
	Amount              int64  `json:"amount"`
}

type BillResponse struct{}

// Bill transfers money from the billing account into a target account.
func (s *Service) Bill(req *BillRequest, resp *BillResponse) error {
	if _, err := s.moneyClient.Transfer(context.Background(), &moneypb.TransferRequest{
		SourceAccountHandle: s.billingAccountHandle,
		SourceAccountKey:    s.billingAccountKey,
		TargetAccountHandle: req.TargetAccountHandle,
		Amount:              req.Amount,
	}); err != nil {
		return err
	}

	s.billingAccountTransfers += req.Amount

	resp = &BillResponse{}
	return nil
}

func (s *Service) Transfers() *Transfers {
	return &Transfers{
		ExecutingAccount: s.executingAccountTransfers,
		BillingAccount:   s.billingAccountTransfers,
	}
}
