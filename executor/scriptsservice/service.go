package scriptsservice

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
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

	nsenternetPath string
	supervisorPath string
	k4LibraryPath  string

	chroot       string
	parentCgroup string

	executionID int64
}

func New(lis net.Listener, scripts *scripts.Store, accounts *accounts.Store, nsenternetPath string, supervisorPath string, k4LibraryPath string, chroot string, parentCgroup string) *Service {
	prometheus.MustRegister(scriptRealExecutionDurationsHistogram)
	prometheus.MustRegister(scriptCPUExecutionDurationsHistogram)
	prometheus.MustRegister(scriptUsesByServer)

	return &Service{
		lis: lis,

		scripts:  scripts,
		accounts: accounts,

		nsenternetPath: nsenternetPath,
		supervisorPath: supervisorPath,
		k4LibraryPath:  k4LibraryPath,

		chroot:       chroot,
		parentCgroup: parentCgroup,

		executionID: 0,
	}
}

func (s *Service) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	script, err := s.scripts.Create(ctx, req.OwnerName, req.Name)
	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrAlreadyExists:
			return nil, grpc.Errorf(codes.AlreadyExists, "script already exists")
		}
		glog.Errorf("Failed to get create script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if err := script.SetMeta(ctx, req.Meta); err != nil {
		script.Delete(context.Background())
		glog.Errorf("Failed to set script meta: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	if err := script.SetContent(ctx, req.Content); err != nil {
		script.Delete(context.Background())
		glog.Errorf("Failed to write to file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to create script")
	}

	return &pb.CreateResponse{}, nil
}

func (s *Service) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	foundScripts, err := s.scripts.Scripts(ctx, req.OwnerName, req.Query, req.ViewerName, req.Offset, req.Limit, req.SortOrder)
	if err != nil {
		if err == scripts.ErrNotFound {
			return nil, grpc.Errorf(codes.NotFound, "account not found")
		}
		glog.Errorf("Failed to list scripts: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to list scripts")
	}

	entries := make([]*pb.ListResponse_Entry, len(foundScripts))
	for i, script := range foundScripts {
		meta, err := script.Meta(ctx)
		if err != nil {
			glog.Errorf("Failed to get script meta: %v", err)
			return nil, grpc.Errorf(codes.Internal, "failed to get script meta")
		}

		entries[i] = &pb.ListResponse_Entry{
			OwnerName: script.OwnerName,
			Name:      script.Name,
			Meta:      meta,
		}
	}

	return &pb.ListResponse{
		Entry: entries,
	}, nil
}

func (s *Service) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	if err := script.Delete(ctx); err != nil {
		glog.Errorf("Failed to delete script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to delete script")
	}

	return &pb.DeleteResponse{}, nil
}

func (s *Service) Vote(ctx context.Context, req *pb.VoteRequest) (*pb.VoteResponse, error) {
	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	if err := script.Vote(ctx, int(req.Delta)); err != nil {
		glog.Errorf("Failed to vote on script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to vote on script")
	}

	return &pb.VoteResponse{}, nil
}

var defaultCgroupSubsystems = []string{"memory", "cpu", "blkio"}

func makeCgroups(subsystems []string, name string) ([]string, error) {
	paths := make([]string, len(subsystems))
	for i, subsystem := range subsystems {
		mountpoint, err := cgroups.FindCgroupMountpoint(subsystem)
		if err != nil {
			return nil, err
		}

		cgroupPath := filepath.Join(mountpoint, name)
		if err := os.MkdirAll(cgroupPath, 0755); err != nil {
			return nil, err
		}

		paths[i] = cgroupPath
	}

	return paths, nil
}

func removeCgroup(path string) error {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}

	for _, f := range files {
		if f.IsDir() {
			if err := removeCgroup(filepath.Join(path, f.Name())); err != nil {
				return err
			}
		}
	}

	glog.Infof("Removing cgroup path: %s", path)
	return os.Remove(path)
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

	traits, err := account.Traits(ctx)
	if err != nil {
		glog.Errorf("Failed to get account traits: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	script, err := s.scripts.Open(ctx, req.OwnerName, req.Name)

	if err != nil {
		switch err {
		case scripts.ErrInvalidName:
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	cgroupName := fmt.Sprintf("%s/execution-%d-%d", s.parentCgroup, time.Now().Unix(), s.executionID)
	s.executionID++
	cgroupPaths, err := makeCgroups(defaultCgroupSubsystems, cgroupName)
	if err != nil {
		glog.Errorf("Failed to open parent file conn: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer func() {
		for _, path := range cgroupPaths {
			if err := removeCgroup(path); err != nil {
				glog.Errorf("Failed to remove all cgroups: %v", err)
			}
		}
	}()

	containersPath, err := ioutil.TempDir("", "kobun4-executor-containers-")
	if err != nil {
		glog.Errorf("Failed to create containers temp directory: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer os.RemoveAll(containersPath)

	rootfsesPath, err := ioutil.TempDir("", "kobun4-executor-rootfses-")
	if err != nil {
		glog.Errorf("Failed to create rootfses temp directory: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer os.RemoveAll(rootfsesPath)

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

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()

	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer statusReader.Close()
	defer statusWriter.Close()

	reqReader, reqWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer reqWriter.Close()
	defer reqReader.Close()

	timeout := time.Duration(traits.TimeLimitSeconds) * 2 * time.Second
	glog.Infof("Starting supervisor with timeout: %s", timeout)

	commandCtx, commandCancel := context.WithTimeout(ctx, timeout)
	defer commandCancel()

	cmd := exec.CommandContext(commandCtx, s.supervisorPath, "-logtostderr", "-parent_cgroup", cgroupName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
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

	wg.Add(1)
	go func() {
		io.Copy(limio.LimitWriter(&stdout, maxBufferSize), stdoutReader)
		stdoutReader.Close()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		io.Copy(limio.LimitWriter(&stderr, maxBufferSize), stderrReader)
		stderrReader.Close()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		io.Copy(&status, statusReader)
		statusReader.Close()
		wg.Done()
	}()

	stdinReader.Close()
	stdoutWriter.Close()
	stderrWriter.Close()
	statusWriter.Close()
	reqReader.Close()

	workerReq := &pb.WorkerExecutionRequest{
		Config: &pb.WorkerExecutionRequest_Configuration{
			ContainersPath: containersPath,
			Chroot:         s.chroot,
			RootfsesPath:   rootfsesPath,

			Hostname: "kobun4",

			StorageRootPath: s.accounts.StorageRootPath(),
			K4LibraryPath:   s.k4LibraryPath,
			NsenternetPath:  s.nsenternetPath,

			BridgeTarget:   req.BridgeTarget,
			ExecutorTarget: s.lis.Addr().String(),
		},
		OwnerName: script.OwnerName,
		Name:      script.Name,

		Context: req.Context,
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

	stdinWriter.Close()
	stdoutReader.Close()
	stderrReader.Close()
	statusReader.Close()

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
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	content, err := script.Content(ctx)
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
			return nil, grpc.Errorf(codes.InvalidArgument, "invalid script name, must only contain numbers and lowercase alphabetical characters")
		case scripts.ErrNotFound:
			return nil, grpc.Errorf(codes.NotFound, "script not found")
		}
		glog.Errorf("Failed to get load script: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to load script")
	}

	reqs, err := script.Meta(ctx)
	if err != nil {
		glog.Errorf("Failed to get meta: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to get meta")
	}

	return &pb.GetMetaResponse{
		Meta: reqs,
	}, nil
}
