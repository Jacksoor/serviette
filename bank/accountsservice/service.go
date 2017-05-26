package accountsservice

import (
	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/bank/accounts"

	pb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
)

type Service struct {
	accounts *accounts.Store
}

func New(accounts *accounts.Store) *Service {
	return &Service{
		accounts: accounts,
	}
}

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	account, err := s.accounts.Create(ctx, tx)

	if err != nil {
		glog.Errorf("Failed to create account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create account")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.CreateResponse{
		AccountHandle: account.Handle(),
		AccountKey:    account.Key(),
	}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	accounts, err := s.accounts.Accounts(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to load accounts: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load accounts")
	}

	handles := make([][]byte, len(accounts))
	for i, account := range accounts {
		handles[i] = account.Handle()
	}

	return &pb.ListResponse{
		AccountHandle: handles,
	}, nil
}

func (s *Service) Check(ctx context.Context, req *pb.CheckRequest) (*pb.CheckResponse, error) {
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

	if string(req.AccountKey) != string(account.Key()) {
		return nil, grpc.Errorf(codes.PermissionDenied, "bad key")
	}

	return &pb.CheckResponse{}, nil
}

func (s *Service) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
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

	if err := account.Delete(ctx, tx); err != nil {
		glog.Errorf("Failed to delete account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete account")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.DeleteResponse{}, nil
}

func (s *Service) ResolveAlias(ctx context.Context, req *pb.ResolveAliasRequest) (*pb.ResolveAliasResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	account, err := s.accounts.LoadByAlias(ctx, tx, req.Name)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to load account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	return &pb.ResolveAliasResponse{
		AccountHandle: account.Handle(),
		AccountKey:    account.Key(),
	}, nil
}

func (s *Service) SetAlias(ctx context.Context, req *pb.SetAliasRequest) (*pb.SetAliasResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	var account *accounts.Account
	if req.AccountHandle != nil {
		account, err = s.accounts.Load(ctx, tx, req.AccountHandle)
		if err != nil {
			if err == accounts.ErrNotFound {
				return nil, grpc.Errorf(codes.NotFound, "account not found")
			}
			glog.Errorf("Failed to load account: %v", err)
			return nil, grpc.Errorf(codes.Internal, "failed to load account")
		}
	}

	if err := s.accounts.SetAlias(ctx, tx, req.Name, account); err != nil {
		glog.Errorf("Failed to set alias: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to set alias")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.SetAliasResponse{}, nil
}
