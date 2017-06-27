package scriptsservice

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/djherbis/buffer/limio"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/budget"
	"github.com/porpoises/kobun4/executor/scripts"

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
	lis net.Listener

	scripts  *scripts.Store
	accounts *accounts.Store

	budgeter *budget.Budgeter

	supervisorPrefix []string
	supervisorPath   string
	k4LibraryPath    string
	containersPath   string
	chroot           string
	parentCgroup     string

	executionID int64
}

func New(lis net.Listener, scripts *scripts.Store, accounts *accounts.Store, budgeter *budget.Budgeter, supervisorPrefix []string, supervisorPath string, k4LibraryPath string, containersPath string, chroot string, parentCgroup string) *Service {
	prometheus.MustRegister(scriptRealExecutionDurationsHistogram)
	prometheus.MustRegister(scriptCPUExecutionDurationsHistogram)
	prometheus.MustRegister(scriptUsesByServer)

	return &Service{
		lis: lis,

		scripts:  scripts,
		accounts: accounts,

		budgeter: budgeter,

		supervisorPrefix: supervisorPrefix,
		supervisorPath:   supervisorPath,
		k4LibraryPath:    k4LibraryPath,
		containersPath:   containersPath,
		chroot:           chroot,
		parentCgroup:     parentCgroup,

		executionID: 0,
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

const (
	defaultUid uint32 = 0
	defaultGid        = 0
)

type singleListener struct {
	conn net.Conn
	once sync.Once
}

func (s *singleListener) Accept() (net.Conn, error) {
	var c net.Conn
	s.once.Do(func() {
		c = s.conn
	})
	if c != nil {
		return c, nil
	}
	return nil, io.EOF
}

func (s *singleListener) Close() error {
	s.once.Do(func() {
		s.conn.Close()
	})
	return nil
}

func (s *singleListener) Addr() net.Addr {
	return s.conn.LocalAddr()
}

func makeCgroup(subsystem string, name string) (string, error) {
	mountpoint, err := cgroups.FindCgroupMountpoint(subsystem)
	if err != nil {
		return "", err
	}

	cgroupPath := filepath.Join(mountpoint, name)
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return "", err
	}

	return cgroupPath, nil
}

func (s *Service) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	remainingBudget, err := s.budgeter.Remaining(ctx, req.Context.NetworkId, req.Context.UserId)
	if err != nil {
		glog.Errorf("Failed to get budget: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get executing user info")
	}

	if remainingBudget <= 0 {
		return nil, grpc.Errorf(codes.ResourceExhausted, "not enough budget")
	}

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

	cgroup := fmt.Sprintf("%s/execution-%d", s.parentCgroup, s.executionID)
	s.executionID++
	memoryCgroupPath, err := makeCgroup("memory", cgroup)
	if err != nil {
		glog.Errorf("Failed to open parent file conn: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer os.RemoveAll(memoryCgroupPath)

	var wg sync.WaitGroup

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var status bytes.Buffer

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stdinWriter.Close()
	defer stdinReader.Close()
	wg.Add(1)
	go func() {
		stdinWriter.Write([]byte(req.Stdin))
		stdinWriter.Close()
		wg.Done()
	}()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	wg.Add(1)
	go func() {
		io.Copy(limio.LimitWriter(&stdout, maxBufferSize), stdoutReader)
		stdoutReader.Close()
		wg.Done()
	}()

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()
	wg.Add(1)
	go func() {
		io.Copy(limio.LimitWriter(&stderr, maxBufferSize), stderrReader)
		stderrReader.Close()
		wg.Done()
	}()

	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	wg.Add(1)
	go func() {
		io.Copy(&status, statusReader)
		statusReader.Close()
		wg.Done()
	}()

	reqReader, reqWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer reqWriter.Close()
	defer reqReader.Close()

	args := append(s.supervisorPrefix, s.supervisorPath, "-logtostderr",
		"-parent_cgroup", cgroup)

	timeout := time.Duration(account.Traits.TimeLimitSeconds) * 2 * time.Second
	glog.Infof("Starting supervisor with timeout: %s", timeout)

	commandCtx, commandCancel := context.WithTimeout(ctx, timeout)
	defer commandCancel()

	cmd := exec.CommandContext(commandCtx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:    true,
		Pdeathsig: syscall.SIGKILL,
	}
	cmd.ExtraFiles = []*os.File{
		stdinReader,
		stdoutWriter,
		stderrWriter,
		statusWriter,
		reqReader,
	}
	if err := cmd.Start(); err != nil {
		glog.Errorf("Failed to start supervisor: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	stdinReader.Close()
	stdoutWriter.Close()
	stderrWriter.Close()
	statusWriter.Close()
	reqReader.Close()

	workerReq := &pb.WorkerExecutionRequest{
		Config: &pb.WorkerExecutionRequest_Configuration{
			ContainersPath: s.containersPath,
			Chroot:         s.chroot,

			Hostname: "kobun4",

			StoragePath:   s.accounts.StoragePath(),
			ScriptsPath:   s.scripts.RootPath(),
			K4LibraryPath: s.k4LibraryPath,

			Uid: defaultUid,
			Gid: defaultGid,
		},
		OwnerName: script.OwnerName,
		Name:      script.Name,

		Context: req.Context,

		BridgeTarget:   req.BridgeTarget,
		ExecutorTarget: s.lis.Addr().String(),
	}
	glog.Infof("Execution request: %s", workerReq)

	rawReq, err := proto.Marshal(workerReq)
	if err != nil {
		glog.Errorf("Failed to marshal request: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	if _, err := reqWriter.Write(rawReq); err != nil {
		glog.Errorf("Failed to write request to supervisor: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	reqWriter.Close()

	if err := cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			glog.Errorf("Failed to get ExitError: %v", err)
			return nil, grpc.Errorf(codes.Internal, "failed to run script")
		}
	}

	wg.Wait()
	rawStatus := status.Bytes()
	if len(rawStatus) == 0 {
		glog.Errorf("No status received?")
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	result := &pb.WorkerExecutionResult{}
	if err := proto.Unmarshal(rawStatus, result); err != nil {
		glog.Errorf("Failed to unmarshal status: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	scriptCPUExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(time.Duration(result.Timings.UserNanos+result.Timings.SystemNanos)*time.Nanosecond) / float64(time.Millisecond))
	scriptRealExecutionDurationsHistogram.WithLabelValues(script.OwnerName, script.Name).Observe(float64(time.Duration(result.Timings.RealNanos)*time.Nanosecond) / float64(time.Millisecond))
	scriptUsesByServer.WithLabelValues(req.Context.BridgeName, req.Context.NetworkId, req.Context.GroupId, script.OwnerName, script.Name).Inc()

	if err := s.budgeter.Charge(ctx, req.Context.NetworkId, req.Context.UserId, time.Duration(result.Timings.RealNanos)*time.Nanosecond); err != nil {
		glog.Errorf("Failed to charge to budget: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	return &pb.ExecuteResponse{
		Result: result,
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
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
