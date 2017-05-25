package scriptsservice

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/pricing"
	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/accountsservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/contextservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	scripts *scripts.Store

	imagesRoot string
	imageSize  int64

	mountsRoot string
	mountsMu   sync.Mutex

	moneyClient    moneypb.MoneyClient
	accountsClient accountspb.AccountsClient

	pricer     pricing.Pricer
	supervisor *worker.Supervisor
}

func New(scripts *scripts.Store, imagesRoot string, imageSize int64, moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, pricer pricing.Pricer, supervisor *worker.Supervisor) (*Service, error) {
	mountsRoot, err := ioutil.TempDir("", "kobun4-mounts-")
	if err != nil {
		return nil, err
	}

	return &Service{
		scripts: scripts,

		imagesRoot: imagesRoot,
		imageSize:  imageSize,

		mountsRoot: mountsRoot,

		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		pricer:     pricer,
		supervisor: supervisor,
	}, nil
}

func (s *Service) Stop() {
	files, err := ioutil.ReadDir(s.mountsRoot)
	if err != nil {
		glog.Errorf("Failed to read mount root %s: %v", s.mountsRoot, err)
		return
	}

	for _, file := range files {
		mountPoint := filepath.Join(s.mountsRoot, file.Name())
		glog.Infof("Unmounting %s", mountPoint)
		if err := exec.Command("fusermount", "-u", mountPoint).Run(); err != nil {
			glog.Errorf("Failed to unmount %s: %v", mountPoint, err)
		} else {
			if err := os.Remove(mountPoint); err != nil {
				glog.Errorf("Failed to remove %s: %v", mountPoint, err)
			}
		}
	}

	if err := os.Remove(s.mountsRoot); err != nil {
		glog.Errorf("Failed to delete root %s: %v", s.mountsRoot, err)
	}
}

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	script, err := s.scripts.Create(req.AccountHandle, req.Name)
	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrAlreadyExists:
			return nil, grpc.Errorf(codes.AlreadyExists, "script already exists")
		}
		glog.Errorf("Failed to get create script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if err := script.SetContent(req.Content); err != nil {
		script.Delete()
		glog.Errorf("Failed to write to file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if err := script.SetRequestedCapabilities(req.RequestedCapabilities); err != nil {
		script.Delete()
		glog.Errorf("Failed to set xattr on file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	return &pb.CreateResponse{}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	scripts, err := s.scripts.AccountScripts(req.AccountHandle)
	if err != nil {
		glog.Errorf("Failed to list scripts: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list scripts")
	}

	names := make([]string, len(scripts))
	for i, script := range scripts {
		names[i] = script.Name()
	}

	return &pb.ListResponse{
		Name: names,
	}, nil
}

func (s *Service) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	script, err := s.scripts.Open(req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	if err := script.Delete(); err != nil {
		glog.Errorf("Failed to delete script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete script")
	}

	return &pb.DeleteResponse{}, nil
}

func (s *Service) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	script, err := s.scripts.Open(req.ScriptAccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	requestedCapabilities, err := script.RequestedCapabilities()
	if err != nil {
		glog.Errorf("Failed to get requested capabilities: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	accountCapabilities, err := script.AccountCapabilities(req.ExecutingAccountHandle)
	if err != nil {
		glog.Errorf("Failed to get account capabilities: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	if requestedCapabilities.WithdrawalLimit > 0 && accountCapabilities.WithdrawalLimit <= 0 {
		return nil, grpc.Errorf(codes.PermissionDenied, "a withdrawal limit was requested but the executing account does not allow withdrawals")
	}

	billingAccountHandle := req.ScriptAccountHandle
	if requestedCapabilities.BillUsageToExecutingAccount {
		if !accountCapabilities.BillUsageToExecutingAccount {
			return nil, grpc.Errorf(codes.PermissionDenied, "the executing account does not allow usage of the script to be billed to them")
		}
		billingAccountHandle = req.ExecutingAccountHandle
	}

	// Ensure disk and mount it.
	mountPath, err := s.ensureAndMountAccountDisk(req.ScriptAccountHandle)
	if err != nil {
		glog.Errorf("Failed to ensure and mount disk: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	nsjailArgs := []string{}
	getBalanceResp, err := s.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: [][]byte{billingAccountHandle},
	})
	if err != nil {
		return nil, err
	}

	balance := getBalanceResp.Balance[0]
	if balance <= 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "the executing account does not have enough funds")
	}

	maxUsage := s.pricer.MaxUsage(balance)
	nsjailArgs = []string{
		"--rlimit_cpu", fmt.Sprintf("%d", maxUsage.RealTime/time.Second),
		"--cgroup_mem_max", fmt.Sprintf("%d", maxUsage.Memory),
		"--bindmount", fmt.Sprintf("%s:/mnt/storage", mountPath),
	}

	worker := s.supervisor.Spawn(script.Path(), []string{}, []byte(req.Rest))

	accountsService := accountsservice.New(s.accountsClient)
	worker.RegisterService("Accounts", accountsService)

	withdrawalLimit := requestedCapabilities.WithdrawalLimit
	if accountCapabilities.WithdrawalLimit < withdrawalLimit {
		withdrawalLimit = accountCapabilities.WithdrawalLimit
	}
	moneyService := moneyservice.New(s.moneyClient, req.ExecutingAccountHandle, req.ExecutingAccountKey, withdrawalLimit)
	worker.RegisterService("Money", moneyService)

	contextService := contextservice.New(req.Context)
	worker.RegisterService("Context", contextService)

	workerCtx, workerCancel := context.WithTimeout(ctx, maxUsage.RealTime)
	defer workerCancel()

	startTime := time.Now()
	r, err := worker.Run(workerCtx, nsjailArgs)
	if r == nil {
		return nil, err
	}
	endTime := time.Now()

	rusage := r.ProcessState.SysUsage().(*syscall.Rusage)

	usageCost := s.pricer.Cost(&pricing.Usage{
		RealTime: endTime.Sub(startTime),
		Memory:   int64(rusage.Maxrss * 1000),
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
	}, nil
}

func (s *Service) GetContent(ctx context.Context, req *pb.GetContentRequest) (*pb.GetContentResponse, error) {
	script, err := s.scripts.Open(req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	content, err := script.Content()
	if err != nil {
		glog.Errorf("Failed to get script content: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get script content")
	}

	return &pb.GetContentResponse{
		Content: content,
	}, nil
}

func (s *Service) GetRequestedCapabilities(ctx context.Context, req *pb.GetRequestedCapabilitiesRequest) (*pb.GetRequestedCapabilitiesResponse, error) {
	script, err := s.scripts.Open(req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	caps, err := script.RequestedCapabilities()
	if err != nil {
		glog.Errorf("Failed to get requested capabilities: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get requested capabilities")
	}

	return &pb.GetRequestedCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (s *Service) GetAccountCapabilities(ctx context.Context, req *pb.GetAccountCapabilitiesRequest) (*pb.GetAccountCapabilitiesResponse, error) {
	script, err := s.scripts.Open(req.ScriptAccountHandle, req.ScriptName)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	caps, err := script.AccountCapabilities(req.ExecutingAccountHandle)
	if err != nil {
		glog.Errorf("Failed to get requested capabilities: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get account capabilities")
	}

	return &pb.GetAccountCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (s *Service) SetAccountCapabilities(ctx context.Context, req *pb.SetAccountCapabilitiesRequest) (*pb.SetAccountCapabilitiesResponse, error) {
	script, err := s.scripts.Open(req.ScriptAccountHandle, req.ScriptName)

	if err != nil {
		switch err {
		case scripts.ErrInvalidScriptName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	if err := script.SetAccountCapabilities(req.ExecutingAccountHandle, req.Capabilities); err != nil {
		glog.Errorf("Failed to get account capabilities: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to set account capabilities")
	}

	return &pb.SetAccountCapabilitiesResponse{}, nil
}

func (s *Service) ensureAndMountAccountDisk(scriptAccountHandle []byte) (string, error) {
	s.mountsMu.Lock()
	defer s.mountsMu.Unlock()

	encodedHandle := base64.RawURLEncoding.EncodeToString(scriptAccountHandle)

	mountPath := filepath.Join(s.mountsRoot, encodedHandle)
	if err := os.Mkdir(mountPath, 0700); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		return mountPath, nil
	}

	imagePath := filepath.Join(s.imagesRoot, encodedHandle)
	if _, err := os.Stat(imagePath); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}

		f, err := os.Create(imagePath)
		if err != nil {
			return "", err
		}

		if err := f.Truncate(s.imageSize); err != nil {
			f.Close()
			return "", err
		}
		f.Close()

		if err := exec.Command("mkfs.ntfs", "-F", imagePath).Run(); err != nil {
			return "", err
		}
	}

	if err := exec.Command("ntfs-3g", imagePath, mountPath).Run(); err != nil {
		return "", err
	}

	return mountPath, nil
}
