package scriptsservice

import (
	"bytes"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/djherbis/buffer/limio"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
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
	scripts  *scripts.Store
	accounts *accounts.Store

	supervisorPrefix []string
	supervisorPath   string
	k4LibraryPath    string
	containersPath   string
	chroot           string
	parentCgroup     string
}

func New(scripts *scripts.Store, accounts *accounts.Store, supervisorPrefix []string, supervisorPath string, k4LibraryPath string, containersPath string, chroot string, parentCgroup string) *Service {
	prometheus.MustRegister(scriptRealExecutionDurationsHistogram)
	prometheus.MustRegister(scriptCPUExecutionDurationsHistogram)
	prometheus.MustRegister(scriptUsesByServer)

	return &Service{
		scripts:  scripts,
		accounts: accounts,

		supervisorPrefix: supervisorPrefix,
		supervisorPath:   supervisorPath,
		k4LibraryPath:    k4LibraryPath,
		containersPath:   containersPath,
		chroot:           chroot,
		parentCgroup:     parentCgroup,
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
	nobodyUid  uint32 = 65534
	nogroupGid        = 65534
)

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
	go func() {
		stdinWriter.Write([]byte(req.Stdin))
		stdinWriter.Close()
	}()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stdoutReader.Close()
	defer stdoutWriter.Close()
	go func() {
		io.Copy(limio.LimitWriter(&stdout, maxBufferSize), stdoutReader)
		stdoutReader.Close()
	}()

	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer stderrReader.Close()
	defer stderrWriter.Close()
	go func() {
		io.Copy(limio.LimitWriter(&stderr, maxBufferSize), stderrReader)
		stderrReader.Close()
	}()

	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer statusReader.Close()
	defer statusWriter.Close()
	go func() {
		io.Copy(limio.LimitWriter(&stdout, maxBufferSize), statusReader)
		statusReader.Close()
	}()

	reqReader, reqWriter, err := os.Pipe()
	if err != nil {
		glog.Errorf("Failed to create pipe: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer reqWriter.Close()
	defer reqReader.Close()

	bridgeConn, err := net.Dial("tcp", req.BridgeTarget)
	if err != nil {
		glog.Errorf("Failed to connect to bridge: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}
	defer bridgeConn.Close()

	bridgeFile, err := bridgeConn.(*net.TCPConn).File()
	if err != nil {
		glog.Errorf("Failed to get bridge file: %v", err)
		return nil, grpc.Errorf(codes.Internal, "failed to run script")
	}

	args := append(s.supervisorPrefix, s.supervisorPath, "-logtostderr",
		"-parent_group", s.parentCgroup)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.ExtraFiles = []*os.File{
		stdinReader,
		stdoutWriter,
		stderrWriter,
		statusWriter,
		reqReader,
		bridgeFile,
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

	rawReq, err := proto.Marshal(&pb.WorkerExecutionRequest{
		Config: &pb.WorkerExecutionRequest_Configuration{
			ContainersPath: s.containersPath,
			Chroot:         s.chroot,

			Hostname: "kobun4",

			PrivateStoragePath: account.StoragePath(),
			ScriptsPath:        s.scripts.RootPath(),
			K4LibraryPath:      s.k4LibraryPath,

			Uid: nobodyUid,
			Gid: nogroupGid,
		},
		OwnerName: script.OwnerName,
		Name:      script.Name,

		Context: req.Context,
		Traits:  account.Traits,
	})
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

	result := &pb.WorkerExecutionResult{}
	if err := proto.Unmarshal(status.Bytes(), result); err != nil {
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
