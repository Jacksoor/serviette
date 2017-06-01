package scriptsservice

import (
	"encoding/base64"
	"fmt"
	"net"
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
	bridgepb "github.com/porpoises/kobun4/executor/bridgeservice/v1pb"

	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/accountsservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/bridgeservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/moneyservice"
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
	minimumUsageCost    int64

	supervisor *worker.Supervisor
}

func New(scripts *scripts.Store, mounter *scripts.Mounter, k4LibraryPath string, moneyClient moneypb.MoneyClient, accountsClient accountspb.AccountsClient, durationPerUnitCost time.Duration, minimumUsageCost int64, supervisor *worker.Supervisor) *Service {
	return &Service{
		scripts: scripts,
		mounter: mounter,

		k4LibraryPath: k4LibraryPath,

		moneyClient:    moneyClient,
		accountsClient: accountsClient,

		durationPerUnitCost: durationPerUnitCost,
		minimumUsageCost:    minimumUsageCost,

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

	if err := script.SetRequirements(req.Requirements); err != nil {
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

	requirements, err := script.Requirements()
	if err != nil {
		glog.Errorf("Failed to get requirements: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	billingAccountHandle := req.ExecutingAccountHandle
	if requirements.BillUsageToOwner {
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

	if getBalanceResp.Balance <= 0 {
		return nil, grpc.Errorf(codes.FailedPrecondition, "the executing account does not have enough funds")
	}

	worker := s.supervisor.Spawn(filepath.Join("/mnt/scripts", script.QualifiedName()), []string{}, []byte(req.Rest))

	accountsService := accountsservice.New(s.accountsClient)
	worker.RegisterService("Accounts", accountsService)

	moneyService := moneyservice.New(s.moneyClient, s.accountsClient, req.ScriptAccountHandle, req.ExecutingAccountHandle, req.EscrowedFunds)
	worker.RegisterService("Money", moneyService)

	scriptContext := req.Context
	scriptContext.ScriptAccountHandle = base64.RawURLEncoding.EncodeToString(req.ScriptAccountHandle)
	scriptContext.BillingAccountHandle = base64.RawURLEncoding.EncodeToString(billingAccountHandle)
	scriptContext.ExecutingAccountHandle = base64.RawURLEncoding.EncodeToString(req.ExecutingAccountHandle)

	outputService := outputservice.New("text")
	worker.RegisterService("Output", outputService)

	bridgeConn, err := grpc.Dial(req.BridgeServiceTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		return nil, grpc.Errorf(codes.Unavailable, "bridge service unavailable")
	}
	defer bridgeConn.Close()

	bridgeService := bridgeservice.New(bridgepb.NewBridgeClient(bridgeConn))
	worker.RegisterService("Bridge", bridgeService)

	workerCtx, workerCancel := context.WithTimeout(ctx, time.Duration(getBalanceResp.Balance)*s.durationPerUnitCost)
	defer workerCancel()

	rawCtx, err := marshaler.MarshalToString(req.Context)
	if err != nil {
		glog.Errorf("Failed to marshal context: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	startTime := time.Now()
	r, err := worker.Run(workerCtx, []string{
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
	endTime := time.Now()

	dur := endTime.Sub(startTime)
	usageCost := int64(dur / s.durationPerUnitCost)
	if usageCost < s.minimumUsageCost {
		usageCost = s.minimumUsageCost
	}
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

func (s *Service) GetRequirements(ctx context.Context, req *pb.GetRequirementsRequest) (*pb.GetRequirementsResponse, error) {
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

	reqs, err := script.Requirements()
	if err != nil {
		glog.Errorf("Failed to get requirements: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get requirements")
	}

	return &pb.GetRequirementsResponse{
		Requirements: reqs,
	}, nil
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
