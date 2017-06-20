package accountsservice

import (
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"time"

	"github.com/porpoises/kobun4/executor/accounts"

	pb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
)

type Service struct {
	accounts *accounts.Store
}

func New(accounts *accounts.Store) *Service {
	return &Service{
		accounts: accounts,
	}
}

func (s *Service) Authenticate(ctx context.Context, req *pb.AuthenticateRequest) (*pb.AuthenticateResponse, error) {
	account, err := s.accounts.Account(ctx, req.Username)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to load account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	if err := account.Authenticate(ctx, req.Password); err != nil {
		if err == accounts.ErrUnauthenticated {
			return nil, grpc.Errorf(codes.PermissionDenied, "invalid credentials")
		}
		glog.Errorf("Failed to authenticate account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to authenticate account")
	}

	return &pb.AuthenticateResponse{}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	names, err := s.accounts.AccountNames(ctx)
	if err != nil {
		glog.Errorf("Failed to list accounts: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list accounts")
	}

	return &pb.ListResponse{
		Name: names,
	}, nil
}

func (s *Service) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	account, err := s.accounts.Account(ctx, req.Username)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to load account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	storageInfo, err := account.StorageInfo()
	if err != nil {
		glog.Errorf("Failed to get account storage info: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	return &pb.GetResponse{
		StorageSize: storageInfo.StorageSize,
		FreeSize:    storageInfo.FreeSize,

		TimeLimitSeconds:   int64(account.TimeLimit / time.Second),
		MemoryLimit:        account.MemoryLimit,
		TmpfsSize:          account.TmpfsSize,
		AllowNetworkAccess: account.AllowNetworkAccess,

		AllowRawOutput: account.AllowRawOutput,

		AllowedService: account.AllowedServices,
	}, nil
}
