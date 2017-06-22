package scriptsservice

import (
	"bytes"
	"fmt"
	"path/filepath"
	"syscall"
	"time"

	"github.com/djherbis/buffer/limio"
	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	statspb "github.com/porpoises/kobun4/executor/statsservice/v1pb"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"
	"github.com/porpoises/kobun4/executor/worker/rpc/messagingservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/networkinfoservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/outputservice"
	"github.com/porpoises/kobun4/executor/worker/rpc/statsservice"

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

const maxBufferSize int64 = 5 * 1024 * 1024 // 5MB

type Service struct {
	scripts  *scripts.Store
	accounts *accounts.Store

	k4LibraryPath string

	workerOptions *WorkerOptions
}

type WorkerOptions struct {
	Chroot             string
	KafelSeccompPolicy string
	Network            *NetworkOptions

	TimeLimit   time.Duration
	MemoryLimit int64
	TmpfsSize   int64
}

type NetworkOptions struct {
	Interface string
	IP        string
	Netmask   string
	Gateway   string
}

func New(scripts *scripts.Store, accounts *accounts.Store, k4LibraryPath string, workerOptions *WorkerOptions) *Service {
	prometheus.MustRegister(scriptRealExecutionDurationsHistogram)
	prometheus.MustRegister(scriptCPUExecutionDurationsHistogram)
	prometheus.MustRegister(scriptUsesByServer)

	return &Service{
		scripts:  scripts,
		accounts: accounts,

		k4LibraryPath: k4LibraryPath,

		workerOptions: workerOptions,
	}
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

type workerServiceFactory func(ctx context.Context, bridgeConn *grpc.ClientConn, account *accounts.Account) (interface{}, error)

var workerServiceFactories map[string]workerServiceFactory = map[string]workerServiceFactory{
	"Messaging": func(ctx context.Context, bridgeConn *grpc.ClientConn, account *accounts.Account) (interface{}, error) {
		return messagingservice.New(ctx, account, messagingpb.NewMessagingClient(bridgeConn)), nil
	},

	"NetworkInfo": func(ctx context.Context, bridgeConn *grpc.ClientConn, account *accounts.Account) (interface{}, error) {
		return networkinfoservice.New(ctx, networkinfopb.NewNetworkInfoClient(bridgeConn)), nil
	},

	"Stats": func(ctx context.Context, bridgeConn *grpc.ClientConn, account *accounts.Account) (interface{}, error) {
		return statsservice.New(ctx, statspb.NewStatsClient(bridgeConn)), nil
	},
}

const rlimitAddressSpaceMB int64 = 1 * 1024 * 1024 * 1024 // 1GB

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

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	nsjailArgs := []string{
		"--user", "nobody",
		"--group", "nogroup",
		"--hostname", "kobun4",

		"--chroot", s.workerOptions.Chroot,

		"--bindmount", fmt.Sprintf("%s:/mnt/storage", account.StoragePath()),
		"--bindmount_ro", fmt.Sprintf("%s:/mnt/scripts", s.scripts.RootPath()),
		"--bindmount_ro", fmt.Sprintf("%s:/usr/lib/k4", s.k4LibraryPath),

		"--cwd", "/mnt/storage",

		"--env", fmt.Sprintf("K4_CONTEXT=%s", rawCtx),

		"--cgroup_mem_max", fmt.Sprintf("%d", s.workerOptions.MemoryLimit),
		"--cgroup_mem_parent", "/",

		"--cgroup_pids_parent", "/",

		"--rlimit_as", fmt.Sprintf("%d", rlimitAddressSpaceMB),

		"--tmpfsmount", "/tmp",
		"--tmpfs_size", fmt.Sprintf("%d", account.TmpfsSize),

		"--seccomp_string", s.workerOptions.KafelSeccompPolicy,
	}

	if account.AllowNetworkAccess {
		nsjailArgs = append(nsjailArgs,
			"--macvlan_iface", s.workerOptions.Network.Interface,
			"--macvlan_vs_ip", s.workerOptions.Network.IP,
			"--macvlan_vs_nm", s.workerOptions.Network.Netmask,
			"--macvlan_vs_gw", s.workerOptions.Network.Gateway,
		)
	}

	w := worker.New(nsjailArgs, filepath.Join("/mnt/scripts", script.QualifiedName()), []string{}, bytes.NewBuffer(req.Stdin), limio.LimitWriter(&stdout, maxBufferSize), limio.LimitWriter(&stderr, maxBufferSize))

	bridgeConn, err := grpc.Dial(req.BridgeTarget, grpc.WithInsecure())
	if err != nil {
		return nil, grpc.Errorf(codes.Unavailable, "Service unavailable")
	}
	defer bridgeConn.Close()

	// Always register the output service.
	outputService := outputservice.New(account)
	w.RegisterService("Output", outputService)

	for _, serviceName := range account.AllowedServices {
		factory, ok := workerServiceFactories[serviceName]
		if !ok {
			glog.Warningf("Unknown service name: %s", serviceName)
			continue
		}

		service, err := factory(ctx, bridgeConn, account)
		if err != nil {
			glog.Errorf("Failed to create service: %v", err)
			return nil, grpc.Errorf(codes.Unavailable, "%s service unavailable", serviceName)
		}

		w.RegisterService(serviceName, service)
	}

	startTime := time.Now()

	processCtx, cancel := context.WithTimeout(ctx, account.TimeLimit)
	defer cancel()

	processState, err := w.Run(processCtx)
	if processState == nil {
		glog.Errorf("Failed to run worker: %v", err)
		return nil, err
	}
	endTime := time.Now()

	cpuTime := processState.UserTime() + processState.SystemTime()
	realTime := endTime.Sub(startTime)

	scriptCPUExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(cpuTime) / float64(time.Millisecond))
	scriptRealExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(realTime) / float64(time.Millisecond))
	scriptUsesByServer.WithLabelValues(req.Context.BridgeName, req.Context.NetworkId, req.Context.GroupId, script.OwnerName, script.Name).Inc()

	waitStatus := processState.Sys().(syscall.WaitStatus)

	glog.Infof("Script execution result: %s, CPU time: %s, real time: %s, wait status: %v", string(stderr.Bytes()), cpuTime, realTime, waitStatus)

	// Exited with signal, so shift it back.
	if waitStatus.ExitStatus() > 100 {
		waitStatus = syscall.WaitStatus(int32(waitStatus.ExitStatus()) - 100)
	}

	return &pb.ExecuteResponse{
		WaitStatus: uint32(waitStatus),
		Stdout:     stdout.Bytes(),
		Stderr:     stderr.Bytes(),

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
