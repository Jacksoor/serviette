package moneyservice

import (
	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/bank/accounts"

	pb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

type Service struct {
	accounts *accounts.Store
}

func New(accounts *accounts.Store) *Service {
	return &Service{
		accounts: accounts,
	}
}

func (s *Service) Add(ctx context.Context, req *pb.AddRequest) (*pb.AddResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	account, err := s.accounts.Load(ctx, tx, req.AccountHandle)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to load account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	if err := account.AddMoney(ctx, tx, req.Amount); err != nil {
		glog.Errorf("Failed to deposit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to deposit")
	}

	if err := s.accounts.RecordTransfer(ctx, tx, nil, account, req.Amount); err != nil {
		glog.Errorf("Failed to record transfer: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to record transfer")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.AddResponse{}, nil
}

func (s *Service) GetBalance(ctx context.Context, req *pb.GetBalanceRequest) (*pb.GetBalanceResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	defer tx.Rollback()

	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}

	account, err := s.accounts.Load(ctx, tx, req.AccountHandle)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to load account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	balance, err := account.Balance(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get balance")
	}

	return &pb.GetBalanceResponse{
		Balance: balance,
	}, nil
}

func (s *Service) Transfer(ctx context.Context, req *pb.TransferRequest) (*pb.TransferResponse, error) {
	if req.Amount < 0 {
		return nil, grpc.Errorf(codes.InvalidArgument, "cannot transfer negative amount")
	}

	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	source, err := s.accounts.Load(ctx, tx, req.SourceAccountHandle)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "source account not found")
		}
		glog.Errorf("Failed to load source account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load source account")
	}

	balance, err := source.Balance(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get source balance: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get source balance")
	}

	if balance-int64(req.Amount) < 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "insufficient funds")
	}

	target, err := s.accounts.Load(ctx, tx, req.TargetAccountHandle)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "target account not found")
		}
		glog.Errorf("Failed to load target account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load target account")
	}

	if err := source.AddMoney(ctx, tx, -req.Amount); err != nil {
		glog.Errorf("Failed to withdraw: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to withdraw")
	}

	if err := target.AddMoney(ctx, tx, req.Amount); err != nil {
		glog.Errorf("Failed to deposit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to deposit")
	}

	if err := s.accounts.RecordTransfer(ctx, tx, source, target, req.Amount); err != nil {
		glog.Errorf("Failed to record transfer: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to record transfer")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.TransferResponse{}, nil
}
