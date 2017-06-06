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

func (s *Service) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
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

	return &pb.GetResponse{
		AccountKey: account.Key(),
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
