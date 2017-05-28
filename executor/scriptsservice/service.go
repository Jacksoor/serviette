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

	durationPerUnitCost time.Duration

	supervisor *worker.Supervisor
}

func New(scripts *scripts.Store, imagesRoot string, imageSize int64, moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, durationPerUnitCost time.Duration, supervisor *worker.Supervisor) (*Service, error) {
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

		durationPerUnitCost: durationPerUnitCost,

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
	script, err := s.scripts.Create(ctx, req.AccountHandle, req.Name)
	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
	scripts, err := s.scripts.AccountScripts(ctx, req.AccountHandle)
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
	script, err := s.scripts.Open(ctx, req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
	script, err := s.scripts.Open(ctx, req.ScriptAccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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

	nsjailArgs = []string{
		"--bindmount", fmt.Sprintf("%s:/mnt/storage", mountPath),
	}

	worker := s.supervisor.Spawn(script.Path(), []string{}, []byte(req.Rest))

	accountsService := accountsservice.New(s.accountsClient)
	worker.RegisterService("Accounts", accountsService)

	withdrawalLimit := requestedCapabilities.WithdrawalLimit
	if accountCapabilities.WithdrawalLimit < withdrawalLimit {
		withdrawalLimit = accountCapabilities.WithdrawalLimit
	}
	moneyService := moneyservice.New(s.moneyClient, s.accountsClient, req.ExecutingAccountHandle, req.ExecutingAccountKey, withdrawalLimit)
	worker.RegisterService("Money", moneyService)

	contextService := contextservice.New(req.Context)
	worker.RegisterService("Context", contextService)

	workerCtx, workerCancel := context.WithTimeout(ctx, time.Duration(balance)*s.durationPerUnitCost)
	defer workerCancel()

	startTime := time.Now()
	r, err := worker.Run(workerCtx, nsjailArgs)
	if r == nil {
		glog.Errorf("Failed to run worker: %v", err)
		return nil, err
	}
	endTime := time.Now()

	dur := endTime.Sub(startTime)
	usageCost := int64(dur / s.durationPerUnitCost)

	glog.Infof("Script execution result: %s, time: %s, cost: %d", string(r.Stderr), dur, usageCost)

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
		Context:    contextService.Context(),
		WaitStatus: uint32(r.ProcessState.Sys().(syscall.WaitStatus)),
		Stdout:     r.Stdout,
		Stderr:     r.Stderr,
		UsageCost:  usageCost,
		Withdrawal: withdrawals,
	}, nil
}

func (s *Service) GetContent(ctx context.Context, req *pb.GetContentRequest) (*pb.GetContentResponse, error) {
	script, err := s.scripts.Open(ctx, req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
	script, err := s.scripts.Open(ctx, req.AccountHandle, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
	script, err := s.scripts.Open(ctx, req.ScriptAccountHandle, req.ScriptName)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
	script, err := s.scripts.Open(ctx, req.ScriptAccountHandle, req.ScriptName)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
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
			if eErr, ok := err.(*exec.ExitError); ok {
				return "", fmt.Errorf("mkfs.ntfs %v: %v", eErr, string(eErr.Stderr))
			}
			return "", fmt.Errorf("mkfs.ntfs: %v", err)
		}
	}

	if err := exec.Command("ntfs-3g", imagePath, mountPath).Run(); err != nil {
		if eErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("ntfs-3g %v: %v", eErr, string(eErr.Stderr))
		}
		return "", fmt.Errorf("ntfs-3g: %v", err)
	}

	return mountPath, nil
}

func (s *Service) ResolveAlias(ctx context.Context, req *pb.ResolveAliasRequest) (*pb.ResolveAliasResponse, error) {
	alias, err := s.scripts.LoadAlias(ctx, req.Name)
	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid alias name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "alias not found")
		}
		glog.Errorf("Failed to resolve alias: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to resolve alias")
	}

	return &pb.ResolveAliasResponse{
		AccountHandle:  alias.AccountHandle,
		ScriptName:     alias.ScriptName,
		ExpiryTimeUnix: alias.ExpiryTime.Unix(),
	}, nil
}

func (s *Service) SetAlias(ctx context.Context, req *pb.SetAliasRequest) (*pb.SetAliasResponse, error) {
	if err := s.scripts.SetAlias(ctx, req.Name, req.AccountHandle, req.ScriptName, time.Unix(req.ExpiryTimeUnix, 0)); err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid alias name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "alias not found")
		}
		glog.Errorf("Failed to set alias: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to set alias")
	}

	return &pb.SetAliasResponse{}, nil
}

func (s *Service) ListAliases(ctx context.Context, req *pb.ListAliasesRequest) (*pb.ListAliasesResponse, error) {
	aliases, err := s.scripts.Aliases(ctx)
	if err != nil {
		glog.Errorf("Failed to list aliases: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list aliases")
	}

	entries := make([]*pb.ListAliasesResponse_Entry, len(aliases))
	for i, alias := range aliases {
		entries[i] = &pb.ListAliasesResponse_Entry{
			Name:           alias.Name,
			AccountHandle:  alias.AccountHandle,
			ScriptName:     alias.ScriptName,
			ExpiryTimeUnix: alias.ExpiryTime.Unix(),
		}
	}

	return &pb.ListAliasesResponse{
		Entry: entries,
	}, nil
}
