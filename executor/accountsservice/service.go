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

func (s *Service) Authenticate(ctx context.Context, req *pb.AuthenticateRequest) (*pb.AuthenticateResponse, error) {
	if err := s.accounts.Authenticate(ctx, req.Username, req.Password); err != nil {
		switch err {
		case accounts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		case accounts.ErrUnauthenticated:
			return nil, grpc.Errorf(codes.PermissionDenied, "invalid credentials")
		}
		glog.Errorf("Failed to authenticate account: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to authenticate account")
	}

	return &pb.AuthenticateResponse{}, nil
}
