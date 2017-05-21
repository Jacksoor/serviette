package namesservice

import (
	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/bank/accounts"

	pb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
)

type Service struct {
	accounts *accounts.Store
}

func New(accounts *accounts.Store) *Service {
	return &Service{
		accounts: accounts,
	}
}

func (s *Service) Buy(ctx context.Context, req *pb.BuyRequest) (*pb.BuyResponse, error) {
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

	balance, err := account.Balance(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get balance")
	}

	def, err := s.accounts.NameType(ctx, tx, req.Type)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name type not found")
		}
		glog.Errorf("Failed to load name type: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name type")
	}

	cost := int64(def.Price * req.Periods)

	if balance-cost < 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "insufficient funds")
	}

	if err := account.AddMoney(ctx, tx, -cost); err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to charge price")
	}

	if _, err := s.accounts.AddName(ctx, tx, req.Type, req.Name, account.Handle(), req.Periods, req.Content); err != nil {
		if err == accounts.ErrNoSuchNameType {
			return nil, grpc.Errorf(codes.InvalidArgument, "no such name type")
		}
		glog.Errorf("Failed to create name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create name")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.BuyResponse{}, nil
}

func (s *Service) GetInfo(ctx context.Context, req *pb.GetInfoRequest) (*pb.GetInfoResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	name, err := s.accounts.Name(ctx, tx, req.Type, req.Name)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name not found")
		}
		glog.Errorf("Failed to load name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name")
	}

	info, err := name.Info(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get info: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get info")
	}

	return &pb.GetInfoResponse{
		Info: info,
	}, nil
}

func (s *Service) GetContent(ctx context.Context, req *pb.GetContentRequest) (*pb.GetContentResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	name, err := s.accounts.Name(ctx, tx, req.Type, req.Name)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name not found")
		}
		glog.Errorf("Failed to load name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name")
	}

	content, err := name.Content(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get content: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get content")
	}

	return &pb.GetContentResponse{
		Content: content,
	}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	infos := make([]*pb.Info, 0)

	names, err := s.accounts.Names(ctx, tx)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name not found")
		}
		glog.Errorf("Failed to load names: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load names")
	}

	for i, name := range names {
		info, err := name.Info(ctx, tx)
		if err != nil {
			glog.Errorf("Failed to get name info: %v", err)
			return nil, grpc.Errorf(codes.Internal, "failed to get name info")
		}

		infos[i] = info
	}

	return &pb.ListResponse{
		Info: infos,
	}, nil
}

func (s *Service) Renew(ctx context.Context, req *pb.RenewRequest) (*pb.RenewResponse, error) {
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

	balance, err := account.Balance(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get balance: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get balance")
	}

	def, err := s.accounts.NameType(ctx, tx, req.Type)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name type not found")
		}
		glog.Errorf("Failed to load name type: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name type")
	}

	cost := int64(def.Price * req.Periods)

	if balance-cost < 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "insufficient funds")
	}

	if err := account.AddMoney(ctx, tx, -cost); err != nil {
		return nil, grpc.Errorf(codes.Internal, "failed to charge price")
	}

	name, err := s.accounts.Name(ctx, tx, req.Type, req.Name)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name not found")
		}
		glog.Errorf("Failed to load name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name")
	}

	if err := name.Renew(ctx, tx, req.Periods); err != nil {
		glog.Errorf("Failed to renew name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to renew name")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.RenewResponse{}, nil
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

	if string(req.AccountKey) != string(account.Key()) {
		return nil, grpc.Errorf(codes.PermissionDenied, "bad key")
	}

	name, err := s.accounts.Name(ctx, tx, req.Type, req.Name)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "name not found")
		}
		glog.Errorf("Failed to load name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load name")
	}

	info, err := name.Info(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get info: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get info")
	}

	if string(info.OwnerAccountHandle) != string(account.Handle()) {
		return nil, grpc.Errorf(codes.PermissionDenied, "not owned by requestor")
	}

	if err := name.Delete(ctx, tx); err != nil {
		glog.Errorf("Failed to delete name: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete name")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.DeleteResponse{}, nil
}

func (s *Service) GetTypes(ctx context.Context, req *pb.GetTypesRequest) (*pb.GetTypesResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	defs, err := s.accounts.NameTypes(ctx, tx)
	if err != nil {
		glog.Errorf("Failed to get types: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get types")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.GetTypesResponse{
		Definition: defs,
	}, nil
}

func (s *Service) SetTypes(ctx context.Context, req *pb.SetTypesRequest) (*pb.SetTypesResponse, error) {
	tx, err := s.accounts.BeginTx(ctx)
	if err != nil {
		glog.Errorf("Failed to start transaction: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to start transaction")
	}
	defer tx.Rollback()

	if err := s.accounts.SetNameTypes(ctx, tx, req.Definition); err != nil {
		glog.Errorf("Failed to set types: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to set types")
	}

	if err := tx.Commit(); err != nil {
		glog.Errorf("Failed to commit: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to commit")
	}

	return &pb.SetTypesResponse{}, nil
}
