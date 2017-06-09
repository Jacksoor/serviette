package scriptsservice

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"

	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/networkinfoservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/outputservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

type Service struct {
	scripts *scripts.Store
	mounter *scripts.Mounter

	k4LibraryPath string

	moneyClient    moneypb.MoneyClient
	accountsClient accountspb.AccountsClient

	durationPerUnitCost time.Duration
	baseUsageCost       int64

	supervisor *worker.Supervisor
}

func New(scripts *scripts.Store, mounter *scripts.Mounter, k4LibraryPath string, moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, durationPerUnitCost time.Duration, baseUsageCost int64, supervisor *worker.Supervisor) *Service {
	return &Service{
		scripts: scripts,
		mounter: mounter,

		k4LibraryPath: k4LibraryPath,

		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		durationPerUnitCost: durationPerUnitCost,
		baseUsageCost:       baseUsageCost,

		supervisor: supervisor,
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

	if err := script.SetMeta(req.Meta); err != nil {
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

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
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

	meta, err := script.Meta()
	if err != nil {
		glog.Errorf("Failed to get meta: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	billingAccountHandle := req.ExecutingAccountHandle
	if meta.BillUsageToOwner {
		billingAccountHandle = req.ScriptAccountHandle
	}

	// Ensure disk and mount it.
	mountPath, err := s.mounter.Mount(req.ScriptAccountHandle)
	if err != nil {
		glog.Errorf("Failed to ensure and mount disk: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	getBalanceResp, err := s.moneyClient.GetBalance(ctx, &moneypb.GetBalanceRequest{
		AccountHandle: billingAccountHandle,
	})
	if err != nil {
		return nil, err
	}

	if getBalanceResp.Balance < s.baseUsageCost {
		return nil, grpc.Errorf(codes.FailedPrecondition, "the executing account does not have enough funds")
	}

	worker := s.supervisor.Spawn(filepath.Join("/mnt/scripts", script.QualifiedName()), []string{}, []byte(req.Rest))

	moneyService := moneyservice.New(s.moneyClient, s.accountsClient, req.ScriptAccountHandle, req.ExecutingAccountHandle, req.EscrowedFunds)
	worker.RegisterService("Money", moneyService)

	scriptContext := req.Context
	scriptContext.ScriptAccountHandle = base64.RawURLEncoding.EncodeToString(req.ScriptAccountHandle)
	scriptContext.BillingAccountHandle = base64.RawURLEncoding.EncodeToString(billingAccountHandle)
	scriptContext.ExecutingAccountHandle = base64.RawURLEncoding.EncodeToString(req.ExecutingAccountHandle)

	outputService := outputservice.New("text")
	worker.RegisterService("Output", outputService)

	networkInfoConn, err := grpc.Dial(req.NetworkInfoServiceTarget, grpc.WithInsecure())
	if err != nil {
		return nil, grpc.Errorf(codes.Unavailable, "network info service unavailable")
	}
	defer networkInfoConn.Close()

	networkInfoService := networkinfoservice.New(networkinfopb.NewNetworkInfoClient(networkInfoConn))
	worker.RegisterService("NetworkInfo", networkInfoService)

	rawCtx, err := marshaler.MarshalToString(req.Context)
	if err != nil {
		glog.Errorf("Failed to marshal context: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	r, err := worker.Run(ctx, []string{
		"--bindmount", fmt.Sprintf("%s:/mnt/storage", mountPath),
		"--bindmount_ro", fmt.Sprintf("%s:/mnt/scripts", s.scripts.RootPath()),
		"--bindmount_ro", fmt.Sprintf("%s:/usr/lib/k4", s.k4LibraryPath),
		"--cwd", "/mnt/storage",
		"--env", fmt.Sprintf("K4_CONTEXT=%s", rawCtx),
	})
	if r == nil {
		glog.Errorf("Failed to run worker: %v", err)
		return nil, err
	}

	dur := r.ProcessState.UserTime() + r.ProcessState.SystemTime()

	usageCost := s.baseUsageCost + int64(dur/s.durationPerUnitCost)

	waitStatus := r.ProcessState.Sys().(syscall.WaitStatus)

	glog.Infof("Script execution result: %s, time: %s, cost: %d, wait status: %v", string(r.Stderr), dur, usageCost, waitStatus)

	if _, err := s.moneyClient.Add(ctx, &moneypb.AddRequest{
		AccountHandle: billingAccountHandle,
		Amount:        -usageCost,
	}); err != nil {
		return nil, err
	}

	chargesMap := moneyService.Charges()
	charges := make([]*pb.ExecuteResponse_Charge, 0, len(chargesMap))

	for target, amount := range chargesMap {
		charges = append(charges, &pb.ExecuteResponse_Charge{
			TargetAccountHandle: []byte(target),
			Amount:              amount,
		})
	}

	// Exited with signal, so shift it back.
	if waitStatus.ExitStatus() > 100 {
		waitStatus = syscall.WaitStatus(int32(waitStatus.ExitStatus()) - 100)
	}

	return &pb.ExecuteResponse{
		WaitStatus: uint32(waitStatus),
		Stdout:     r.Stdout,
		Stderr:     r.Stderr,

		UsageCost: usageCost,

		Charge:        charges,
		EscrowedFunds: moneyService.EscrowedFunds(),

		OutputFormat: outputService.Format(),
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

func (s *Service) GetMeta(ctx context.Context, req *pb.GetMetaRequest) (*pb.GetMetaResponse, error) {
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

	reqs, err := script.Meta()
	if err != nil {
		glog.Errorf("Failed to get meta: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get meta")
	}

	return &pb.GetMetaResponse{
		Meta: reqs,
	}, nil
}
