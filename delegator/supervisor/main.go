package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	statspb "github.com/porpoises/kobun4/executor/statsservice/v1pb"

	"github.com/porpoises/kobun4/delegator/supervisor/rpc/messagingservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/networkinfoservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/outputservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/statsservice"
	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	parentCgroup = flag.String("parent_cgroup", "kobun4", "Parent cgroup")
)

type serviceFactory func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error)

type serviceParams struct {
	bridgeConn *grpc.ClientConn
}

var serviceFactories map[string]serviceFactory = map[string]serviceFactory{
	"Messaging": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return messagingservice.New(ctx, account, messagingpb.NewMessagingClient(params.bridgeConn)), nil
	},

	"NetworkInfo": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return networkinfoservice.New(ctx, networkinfopb.NewNetworkInfoClient(params.bridgeConn)), nil
	},

	"Stats": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return statsservice.New(ctx, statspb.NewStatsClient(params.bridgeConn)), nil
	},
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var (
	scriptMountDir         string = "/mnt/scripts"
	privateStorageMountDir        = "/mnt/storage"
	k4LibraryMountDir             = "/usr/lib/k4"
)

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()

		factory, err := libcontainer.New("")
		if err != nil {
			panic(err)
		}

		if err := factory.StartInitialization(); err != nil {
			panic(err)
		}
	}
}

const cgroupMemoryLimit = "memory.limit_in_bytes"

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

func main() {
	flag.Parse()

	childStdin := os.NewFile(3, "child stdin")
	childStdout := os.NewFile(4, "child stdout")
	childStderr := os.NewFile(5, "child stderr")
	childStatus := os.NewFile(6, "child status")
	supervisorRequestFile := os.NewFile(7, "supervisor execution request")
	bridgeConnFile := os.NewFile(8, "bridge connection")

	rawReq, err := ioutil.ReadAll(supervisorRequestFile)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	req := &scriptspb.WorkerExecutionRequest{}
	if err := proto.Unmarshal(rawReq, req); err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	if !nameRegexp.MatchString(req.OwnerName) || !nameRegexp.MatchString(req.Name) {
		glog.Error("invalid owner or script name")
		os.Exit(1)
	}

	if filepath.Dir(filepath.Join(scriptMountDir, req.Name)) != scriptMountDir {
		glog.Error("invalid path")
		os.Exit(1)
	}

	factory, err := libcontainer.New(req.Config.ContainersPath, libcontainer.Cgroupfs, libcontainer.InitArgs(os.Args[0], "init"))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	ctx := context.Background()

	rootfsPath, err := ioutil.TempDir("", "kobun4-supervisor-rootfs-")
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	defer os.RemoveAll(rootfsPath)

	memoryCgroupPath, err := makeCgroup("memory", filepath.Join(*parentCgroup, strconv.Itoa(os.Getpid())))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	defer os.Remove(memoryCgroupPath)

	if req.Traits.MemoryLimit >= 0 {
		if err := ioutil.WriteFile(filepath.Join(memoryCgroupPath, cgroupMemoryLimit), []byte(strconv.FormatInt(req.Traits.MemoryLimit, 10)), 0700); err != nil {
			glog.Error(err)
			os.Exit(1)
		}
	}

	config := &configs.Config{
		Rootfs:     rootfsPath,
		Rootless:   true,
		Readonlyfs: true,
		Capabilities: &configs.Capabilities{
			Bounding:    []string{},
			Effective:   []string{},
			Inheritable: []string{},
			Permitted:   []string{},
			Ambient:     []string{},
		},
		Cgroups: &configs.Cgroup{
			Paths: map[string]string{
				"memory": memoryCgroupPath,
			},
		},
		Namespaces: configs.Namespaces([]configs.Namespace{
			{Type: configs.NEWNS},
			{Type: configs.NEWIPC},
			{Type: configs.NEWPID},
			{Type: configs.NEWUSER},
			{Type: configs.NEWUTS},
		}),
		Devices:  configs.DefaultAutoCreatedDevices,
		Hostname: req.Config.Hostname,
		Mounts: []*configs.Mount{
			{
				Device:      "bind",
				Source:      req.Config.Chroot,
				Destination: "/",
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC | unix.MS_RDONLY,
			},
			{
				Device:      "proc",
				Source:      "proc",
				Destination: "/proc",
				Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV,
			},
			{
				Device:      "tmpfs",
				Source:      "tmpfs",
				Destination: "/dev",
				Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_STRICTATIME,
			},
			{
				Device:      "bind",
				Source:      req.Config.PrivateStoragePath,
				Destination: privateStorageMountDir,
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC,
			},
			{
				Device:      "bind",
				Source:      req.Config.ScriptPath,
				Destination: scriptMountDir,
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC | unix.MS_RDONLY,
			},
			{
				Device:      "bind",
				Source:      req.Config.K4LibraryPath,
				Destination: k4LibraryMountDir,
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC | unix.MS_RDONLY,
			},
		},
		UidMappings: []configs.IDMap{
			{
				ContainerID: int(req.Config.Uid),
				HostID:      os.Geteuid(),
				Size:        1,
			},
		},
		GidMappings: []configs.IDMap{
			{
				ContainerID: int(req.Config.Gid),
				HostID:      os.Getegid(),
				Size:        1,
			},
		},
		Rlimits: []configs.Rlimit{},
	}

	if !req.Traits.AllowNetworkAccess {
		config.Namespaces = append(config.Namespaces, configs.Namespace{Type: configs.NEWNET})
		config.Networks = []*configs.Network{
			{
				Type:    "loopback",
				Address: "127.0.0.1/0",
				Gateway: "localhost",
			},
		}
	}

	if req.Traits.TmpfsSize > 0 {
		config.Mounts = append(config.Mounts, &configs.Mount{
			Device:      "tmpfs",
			Source:      "tmpfs",
			Destination: "/tmp",
			Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_NODEV,
			Data:        fmt.Sprintf("size=%d", req.Traits.TmpfsSize),
		})
	}

	container, err := factory.Create(fmt.Sprintf("%d", os.Getpid()), config)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	defer container.Destroy()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	bridgeConn, err := grpc.Dial("", grpc.WithInsecure(), grpc.WithDialer(func(string, time.Duration) (net.Conn, error) {
		return net.FileConn(bridgeConnFile)
	}))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	parentFile := os.NewFile(uintptr(fds[0]), "")
	defer parentFile.Close()

	rpcServer := rpc.NewServer()

	outputService := outputservice.New(nil)
	rpcServer.RegisterName("Output", outputService)

	params := serviceParams{
		bridgeConn: bridgeConn,
	}

	for _, serviceName := range req.Traits.AllowedService {
		factory, ok := serviceFactories[serviceName]
		if !ok {
			glog.Warningf("Unknown service name: %s", serviceName)
			continue
		}

		service, err := factory(ctx, req.Traits, params)
		if err != nil {
			glog.Fatalf("Failed to create service: %v", err)
		}

		rpcServer.RegisterName(serviceName, service)
	}

	go rpcServer.ServeCodec(jsonrpc.NewServerCodec(parentFile))

	childFile := os.NewFile(uintptr(fds[1]), "")

	jsonK4Context, err := marshaler.MarshalToString(req.Context)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	process := &libcontainer.Process{
		Args: []string{
			filepath.Join(scriptMountDir, req.OwnerName, req.Name),
		},
		Env: []string{
			fmt.Sprintf("K4_CONTEXT=%s", jsonK4Context),
		},
		Stdin:  childStdin,
		Stdout: childStdout,
		Stderr: childStderr,
		ExtraFiles: []*os.File{
			childFile,
		},
	}

	_ = childStdin
	_ = childStdout
	_ = childStderr

	if err := container.Run(process); err != nil {
		childFile.Close()
		glog.Error(err)
		os.Exit(1)
	}
	childFile.Close()

	done := make(chan struct{})
	timeLimitExceeded := false

	go func() {
		select {
		case <-time.After(time.Second):
			process.Signal(os.Kill)
			timeLimitExceeded = true
		case <-done:
		}
	}()

	state, err := process.Wait()
	if state == nil {
		glog.Error(err)
		os.Exit(1)
	}
	close(done)

	waitStatus := state.Sys().(syscall.WaitStatus)
	glog.Infof("Wait status: %d", waitStatus)

	raw, err := proto.Marshal(&scriptspb.WorkerExecutionResult{
		WaitStatus:        uint32(waitStatus),
		TimeLimitExceeded: timeLimitExceeded,
		OutputParams:      outputService.OutputParams,
	})
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	if _, err := childStatus.Write(raw); err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	if err := childStatus.Close(); err != nil {
		glog.Error(err)
		os.Exit(1)
	}
}
