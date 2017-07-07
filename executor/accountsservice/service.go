package accountsservice

import (
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

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

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	if err := s.accounts.Create(ctx, req.Username, req.Password); err != nil {
		switch err {
		case accounts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid account name")
		case accounts.ErrAlreadyExists:
			return nil, grpc.Errorf(codes.AlreadyExists, "already exists")
		}
		glog.Errorf("Failed to create account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create account")
	}

	return &pb.CreateResponse{}, nil
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
	names, err := s.accounts.AccountNames(ctx, req.Offset, req.Limit)
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

	scriptsStorageUsage, err := account.ScriptsStorageUsage()
	if err != nil {
		glog.Errorf("Failed to get scripts storage usage: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	privateStorageUsage, err := account.PrivateStorageUsage()
	if err != nil {
		glog.Errorf("Failed to get private storage usage: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	traits, err := account.Traits(ctx)
	if err != nil {
		glog.Errorf("Failed to get traits: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load account")
	}

	return &pb.GetResponse{
		ScriptsStorageUsage: scriptsStorageUsage,
		PrivateStorageUsage: privateStorageUsage,
		Traits:              traits,
	}, nil
}

func (s *Service) CheckAccountIdentifier(ctx context.Context, req *pb.CheckAccountIdentifierRequest) (*pb.CheckAccountIdentifierResponse, error) {
	if err := s.accounts.CheckAccountIdentifier(ctx, req.Username, req.Identifier); err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to check account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to check account")
	}

	return &pb.CheckAccountIdentifierResponse{}, nil
}
