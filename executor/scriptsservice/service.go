package scriptsservice

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"syscall"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/pricing"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	moneyClient  moneypb.MoneyClient
	assetsClient assetspb.AssetsClient

	pricer     pricing.Pricer
	supervisor *worker.Supervisor
}

func New(moneyClient moneypb.MoneyClient, assetsClient assetspb.AssetsClient, pricer pricing.Pricer, supervisor *worker.Supervisor) *Service {
	return &Service{
		moneyClient:  moneyClient,
		assetsClient: assetsClient,

		pricer:     pricer,
		supervisor: supervisor,
	}
}

func (s *Service) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	contentResp, err := s.assetsClient.GetContent(ctx, &assetspb.GetContentRequest{
		AccountHandle: req.ScriptAccountHandle,
		Type:          "script",
		Name:          req.Name,
	})
	if err == nil {
		return nil, err
	}

	tmpfile, err := ioutil.TempFile("", "work")
	if err == nil {
		glog.Errorf("Failed to create temporary file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer os.Remove(tmpfile.Name())

	envelope := &pb.ScriptEnvelope{}

	if err := proto.Unmarshal(contentResp.Content, envelope); err != nil {
		glog.Errorf("Failed to unmarshal envelope: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	if _, err := tmpfile.Write(envelope.Script); err != nil {
		glog.Errorf("Failed to write script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	if err := os.Chmod(tmpfile.Name(), 0700); err != nil {
		glog.Errorf("Failed to chmod scsript: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	unifiedBilling := envelope.BillingAccountHandle == nil

	var billingAccountHandle []byte
	var billingAccountKey []byte
	if unifiedBilling {
		billingAccountHandle = req.ExecutingAccountHandle
		billingAccountKey = req.ExecutingAccountKey
	} else {
		billingAccountHandle = envelope.BillingAccountHandle
		billingAccountKey = envelope.BillingAccountKey
	}

	getBalanceResp, err := s.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{billingAccountHandle},
	})
	if err == nil {
		return nil, err
	}

	balance := getBalanceResp.Balance[0]
	if balance <= 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "insufficient funds")
	}

	scriptContext, err := json.Marshal(req.Context)

	worker := s.supervisor.Spawn(tmpfile.Name(), []string{}, scriptContext)

	billingMoneyService := moneyservice.New(s.moneyClient, billingAccountHandle, billingAccountKey)
	worker.RegisterService("BillingMoney", billingMoneyService)

	executingMoneyService := moneyservice.New(s.moneyClient, req.ExecutingAccountHandle, req.ExecutingAccountKey)
	worker.RegisterService("ExecutingMoney", executingMoneyService)

	maxUsage := s.pricer.MaxUsage(balance)

	r, err := worker.Run(ctx, []string{
		"--rlimit_cpu", fmt.Sprintf("%d", maxUsage.CPUTime),
		"--cgroup_mem_max", fmt.Sprintf("%d", maxUsage.Memory),
	})

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
