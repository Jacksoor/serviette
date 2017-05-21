package scriptsservice

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"syscall"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/pricing"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	moneyClient moneypb.MoneyClient
	pricer      pricing.Pricer
	supervisor  *worker.Supervisor
	scriptRoot  string
}

func New(scriptRoot string, moneyClient moneypb.MoneyClient, pricer pricing.Pricer, supervisor *worker.Supervisor) *Service {
	return &Service{
		scriptRoot:  scriptRoot,
		moneyClient: moneyClient,
		pricer:      pricer,
		supervisor:  supervisor,
	}
}

func (s *Service) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	accountRoot := filepath.Join(s.scriptRoot, hex.EncodeToString(req.ScriptAccountHandle))
	scriptPath := filepath.Join(accountRoot, req.Name)
	scriptName, err := filepath.Rel(accountRoot, scriptPath)
	if err != nil {
		glog.Errorf("Failed to get relative path: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	if scriptName != req.Name {
		return nil, grpc.Errorf(codes.NotFound, "script not found")
	}

	unifiedBilling := false

	var billingAccountHandle []byte
	var billingAccountKey []byte
	billingAccountHandle = req.ExecutingAccountHandle
	billingAccountKey = req.ExecutingAccountKey

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

	scriptContext, err := json.Marshal(req.Context)

	worker := s.supervisor.Spawn(scriptPath, []string{}, scriptContext)

	billingMoneyService := moneyservice.New(s.moneyClient, billingAccountHandle, billingAccountKey)
	worker.RegisterService("BillingMoney", billingMoneyService)

	executingMoneyService := moneyservice.New(s.moneyClient, req.ExecutingAccountHandle, req.ExecutingAccountKey)
	worker.RegisterService("ExecutingMoney", executingMoneyService)

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

	return &pb.ExecuteResponse{
		Ok:     err == nil,
		Stdout: r.Stdout,
		Stderr: r.Stderr,

		ExecutingAccountTransfers: executingMoneyService.Transfers(),
		BillingAccountTransfers:   billingMoneyService.Transfers(),
		UsageCost:                 usageCost,

		UnifiedBilling: unifiedBilling,
	}, nil
}
