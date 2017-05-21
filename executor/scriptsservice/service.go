package scriptsservice

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/pricing"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/contextservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	scriptRoot string

	moneyClient    moneypb.MoneyClient
	accountsClient accountspb.AccountsClient

	pricer     pricing.Pricer
	supervisor *worker.Supervisor
}

func New(scriptRoot string, moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, pricer pricing.Pricer, supervisor *worker.Supervisor) *Service {
	return &Service{
		scriptRoot: scriptRoot,

		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		pricer:     pricer,
		supervisor: supervisor,
	}
}

var errInvalidScriptName = errors.New("invalid script name")

func (s *Service) scriptPath(accountHandle []byte, scriptName string) (string, error) {
	accountRoot := filepath.Join(s.scriptRoot, base64.URLEncoding.EncodeToString(accountHandle))
	path := filepath.Join(accountRoot, scriptName)
	expectedScriptName, err := filepath.Rel(accountRoot, path)

	if err != nil {
		return "", err
	}

	if scriptName != expectedScriptName {
		return "", errInvalidScriptName
	}

	return path, nil
}

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	accountRoot := filepath.Join(s.scriptRoot, base64.URLEncoding.EncodeToString(req.AccountHandle))
	if err := os.MkdirAll(accountRoot, 0700); err != nil {
		glog.Errorf("Failed to make directories: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	scriptPath, err := s.scriptPath(req.AccountHandle, req.Name)
	if err != nil {
		if err == errInvalidScriptName {
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		}
		glog.Errorf("Failed to get relative path: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if _, err := s.accountsClient.Check(ctx, &accountspb.CheckRequest{
		AccountHandle: req.AccountHandle,
		AccountKey:    req.AccountKey,
	}); err != nil {
		return nil, err
	}

	f, err := os.Create(scriptPath)
	if err != nil {
		glog.Errorf("Failed to create file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if _, err := f.Write(req.Content); err != nil {
		os.Remove(f.Name())
		glog.Errorf("Failed to write to file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if err := f.Chmod(0700); err != nil {
		os.Remove(f.Name())
		glog.Errorf("Failed to chmod file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	return &pb.CreateResponse{}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	accountRoot := filepath.Join(s.scriptRoot, base64.URLEncoding.EncodeToString(req.AccountHandle))
	infos, err := ioutil.ReadDir(accountRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return &pb.ListResponse{}, nil
		}
		glog.Errorf("Failed to read directory: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list scripts")
	}

	names := make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name()
	}

	return &pb.ListResponse{
		Name: names,
	}, nil
}

func (s *Service) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	scriptPath, err := s.scriptPath(req.AccountHandle, req.Name)
	if err != nil {
		if err == errInvalidScriptName {
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		}
		glog.Errorf("Failed to get relative path: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete script")
	}

	if _, err := s.accountsClient.Check(ctx, &accountspb.CheckRequest{
		AccountHandle: req.AccountHandle,
		AccountKey:    req.AccountKey,
	}); err != nil {
		return nil, err
	}

	if err := os.Remove(scriptPath); err != nil {
		if os.IsNotExist(err) {
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to remove script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete script")
	}

	return &pb.DeleteResponse{}, nil
}

func (s *Service) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	scriptPath, err := s.scriptPath(req.ScriptAccountHandle, req.Name)
	if err != nil {
		if err == errInvalidScriptName {
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		}
		glog.Errorf("Failed to get relative path: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	if _, err := os.Stat(scriptPath); err != nil {
		if os.IsNotExist(err) {
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to stat script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	billOwner := false

	var billingAccountHandle []byte
	if billOwner {
		billingAccountHandle = req.ScriptAccountHandle
	} else {
		billingAccountHandle = req.ExecutingAccountHandle
	}

	getBalanceResp, err := s.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{billingAccountHandle},
	})
	if err != nil {
		return nil, err
	}

	balance := getBalanceResp.Balance[0]
	if balance <= 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "insufficient funds")
	}

	worker := s.supervisor.Spawn(scriptPath, []string{}, []byte(req.Rest))

	moneyService := moneyservice.New(s.moneyClient, req.ExecutingAccountHandle, req.ExecutingAccountKey)
	worker.RegisterService("Money", moneyService)

	contextService := contextservice.New(req.Context)
	worker.RegisterService("Context", contextService)

	maxUsage := s.pricer.MaxUsage(balance)

	r, err := worker.Run(ctx, []string{
		"--rlimit_cpu", fmt.Sprintf("%d", maxUsage.CPUTime),
		"--cgroup_mem_max", fmt.Sprintf("%d", maxUsage.Memory),
	})
	if r == nil {
		return nil, err
	}

	rusage := r.ProcessState.SysUsage().(*syscall.Rusage)

	usageCost := s.pricer.Cost(&pricing.Usage{
		CPUTime: r.ProcessState.UserTime() + r.ProcessState.SystemTime(),
		Memory:  int64(rusage.Maxrss * 1000),
	})

	if _, err := s.moneyClient.Add(ctx, &moneypb.AddRequest{
		AccountHandle: billingAccountHandle,
		Amount:        -usageCost,
	}); err != nil {
		return nil, err
	}

	withdrawalMap := moneyService.Withdrawals()
	withdrawals := make([]*pb.ExecuteResponse_Withdrawal, 0, len(withdrawalMap))

	for target, amount := range withdrawalMap {
		withdrawals = append(withdrawals, &pb.ExecuteResponse_Withdrawal{
			TargetAccountHandle: []byte(target),
			Amount:              amount,
		})
	}

	return &pb.ExecuteResponse{
		Ok:         err == nil,
		Stdout:     r.Stdout,
		Stderr:     r.Stderr,
		UsageCost:  usageCost,
		Withdrawal: withdrawals,
		BillOwner:  billOwner,
	}, nil
}
