package scriptsservice

import (
	"bytes"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/messagingservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/networkinfoservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/outputservice"

	pb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	scriptRealExecutionDurationsHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "kobun4",
		Subsystem: "executor",
		Name:      "script_real_execution_durations_histogram_milliseconds",
		Help:      "Script real execution time distributions.",
		Buckets:   prometheus.LinearBuckets(0, 1000, 10),
	}, []string{"owner_name", "script_name"})

	scriptCPUExecutionDurationsHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "kobun4",
		Subsystem: "executor",
		Name:      "script_cpu_execution_durations_histogram_milliseconds",
		Help:      "Script CPU execution time distributions.",
		Buckets:   prometheus.LinearBuckets(0, 50, 10),
	}, []string{"owner_name", "script_name"})

	scriptUsesByServer = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "kobun4",
		Subsystem: "executor",
		Name:      "script_uses_by_server_total",
		Help:      "Script uses by server.",
	}, []string{"bridge_name", "network_id", "group_id", "owner_name", "script_name"})
)

type Service struct {
	scripts  *scripts.Store
	accounts *accounts.Store

	storageRootPath string
	k4LibraryPath   string

	baseWorkerOptions *worker.Options
}

func New(scripts *scripts.Store, accounts *accounts.Store, storageRootPath string, k4LibraryPath string, baseWorkerOptions *worker.Options) (*Service, error) {
	prometheus.MustRegister(scriptRealExecutionDurationsHistogram)
	prometheus.MustRegister(scriptCPUExecutionDurationsHistogram)
	prometheus.MustRegister(scriptUsesByServer)

	storageRootPath, err := filepath.Abs(storageRootPath)
	if err != nil {
		return nil, err
	}

	return &Service{
		scripts:  scripts,
		accounts: accounts,

		storageRootPath: storageRootPath,
		k4LibraryPath:   k4LibraryPath,

		baseWorkerOptions: baseWorkerOptions,
	}, nil
}

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	script, err := s.scripts.Create(ctx, req.OwnerName, req.Name)
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
	accountScripts, err := s.scripts.AccountScripts(ctx, req.OwnerName)
	if err != nil {
		if err == scripts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to list scripts: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list scripts")
	}

	names := make([]string, len(accountScripts))
	for i, script := range accountScripts {
		names[i] = script.Name
	}

	return &pb.ListResponse{
		Name: names,
	}, nil
}

func (s *Service) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

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
	account, err := s.accounts.Account(ctx, req.OwnerName)
	if err != nil {
		if err == accounts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "owning account not found")
		}
		glog.Errorf("Failed to get script owner: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	rawCtx, err := marshaler.MarshalToString(req.Context)
	if err != nil {
		glog.Errorf("Failed to marshal context: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	workerOpts := &worker.Options{}
	*workerOpts = *s.baseWorkerOptions
	workerOpts.TimeLimit = account.TimeLimit
	workerOpts.MemoryLimit = account.MemoryLimit
	workerOpts.TmpfsSize = account.TmpfsSize
	if !account.AllowNetworkAccess {
		workerOpts.Network = nil
	}

	workerOpts.ExtraNsjailArgs = append(workerOpts.ExtraNsjailArgs,
		"--bindmount", fmt.Sprintf("%s:/mnt/storage", filepath.Join(s.storageRootPath, script.OwnerName)),
		"--bindmount_ro", fmt.Sprintf("%s:/mnt/scripts", s.scripts.RootPath()),
		"--bindmount_ro", fmt.Sprintf("%s:/usr/lib/k4", s.k4LibraryPath),
		"--cwd", "/mnt/storage",
		"--env", fmt.Sprintf("K4_CONTEXT=%s", rawCtx),
	)

	w := worker.New(workerOpts, filepath.Join("/mnt/scripts", script.QualifiedName()), []string{}, bytes.NewBuffer(req.Stdin))

	outputService := outputservice.New(account)
	w.RegisterService("Output", outputService)

	networkInfoConn, err := grpc.Dial(req.NetworkInfoServiceTarget, grpc.WithInsecure())
	if err != nil {
		return nil, grpc.Errorf(codes.Unavailable, "network info service unavailable")
	}
	defer networkInfoConn.Close()

	networkInfoService := networkinfoservice.New(ctx, networkinfopb.NewNetworkInfoClient(networkInfoConn))
	w.RegisterService("NetworkInfo", networkInfoService)

	if account.AllowMessagingService {
		messagingConn, err := grpc.Dial(req.MessagingServiceTarget, grpc.WithInsecure())
		if err != nil {
			return nil, grpc.Errorf(codes.Unavailable, "network info service unavailable")
		}
		defer messagingConn.Close()

		messagingService := messagingservice.New(ctx, account, messagingpb.NewMessagingClient(messagingConn))
		w.RegisterService("Messaging", messagingService)
	}

	startTime := time.Now()
	r, err := w.Run(ctx)
	if r == nil {
		glog.Errorf("Failed to run worker: %v", err)
		return nil, err
	}
	endTime := time.Now()

	cpuTime := r.ProcessState.UserTime() + r.ProcessState.SystemTime()
	realTime := endTime.Sub(startTime)

	scriptCPUExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(cpuTime) / float64(time.Millisecond))
	scriptRealExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(realTime) / float64(time.Millisecond))
	scriptUsesByServer.WithLabelValues(req.Context.BridgeName, req.Context.NetworkId, req.Context.GroupId, script.OwnerName, script.Name).Inc()

	waitStatus := r.ProcessState.Sys().(syscall.WaitStatus)

	glog.Infof("Script execution result: %s, CPU time: %s, real time: %s, wait status: %v", string(r.Stderr), cpuTime, realTime, waitStatus)

	// Exited with signal, so shift it back.
	if waitStatus.ExitStatus() > 100 {
		waitStatus = syscall.WaitStatus(int32(waitStatus.ExitStatus()) - 100)
	}

	return &pb.ExecuteResponse{
		WaitStatus: uint32(waitStatus),
		Stdout:     r.Stdout,
		Stderr:     r.Stderr,

		OutputParams: outputService.OutputParams,
	}, nil
}

func (s *Service) GetContent(ctx context.Context, req *pb.GetContentRequest) (*pb.GetContentResponse, error) {
	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

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
	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

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
