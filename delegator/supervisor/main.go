package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/rpc"
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
	"github.com/kballard/go-shellquote"
	"golang.org/x/net/context"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/cgroups"
	"github.com/opencontainers/runc/libcontainer/configs"
	_ "github.com/opencontainers/runc/libcontainer/nsenter"

	"github.com/porpoises/kobun4/delegator/supervisor/seccomp"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	statspb "github.com/porpoises/kobun4/executor/statsservice/v1pb"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/messagingservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/networkinfoservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/outputservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/statsservice"
	"github.com/porpoises/kobun4/delegator/supervisor/rpc/supervisorservice"
	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	parentCgroup = flag.String("parent_cgroup", "kobun4", "Parent cgroup")
)

type serviceFactory func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error)

type serviceParams struct {
	bridgeConn   *grpc.ClientConn
	executorConn *grpc.ClientConn

	currentCgroup string

	req *scriptspb.WorkerExecutionRequest
}

var serviceFactories map[string]serviceFactory = map[string]serviceFactory{
	"Messaging": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return messagingservice.New(ctx, account, messagingpb.NewMessagingClient(params.bridgeConn)), nil
	},

	"NetworkInfo": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return networkinfoservice.New(ctx, params.req.Context, networkinfopb.NewNetworkInfoClient(params.bridgeConn)), nil
	},

	"Stats": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return statsservice.New(ctx, statspb.NewStatsClient(params.bridgeConn)), nil
	},

	"Supervisor": func(ctx context.Context, account *accountspb.Traits, params serviceParams) (interface{}, error) {
		return supervisorservice.New(ctx, params.currentCgroup, params.req.OwnerName, scriptspb.NewScriptsClient(params.executorConn), params.req.Config, params.req.Context), nil
	},
}

var nameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var (
	scriptMountDir        string = "/mnt/scripts"
	privateMountDir              = "/mnt/private"
	legacyPrivateMountDir        = "/mnt/storage"
	k4LibraryMountDir            = "/usr/lib/k4"
)

var marshaler = jsonpb.Marshaler{
	EmitDefaults: true,
}

func init() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runtime.GOMAXPROCS(1)
		runtime.LockOSThread()

		if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0); err != nil {
			os.Exit(1)
		}

		factory, err := libcontainer.New("")
		if err != nil {
			os.Exit(1)
		}

		if err := factory.StartInitialization(); err != nil {
			os.Exit(1)
		}
	}
}

func applyRlimits(traits *accountspb.Traits) {
	syscall.Setrlimit(unix.RLIMIT_AS, &syscall.Rlimit{Cur: uint64(1 * 1024 * 1024 * 1024), Max: uint64(1 * 1024 * 1024 * 1024)})
	syscall.Setrlimit(unix.RLIMIT_CORE, &syscall.Rlimit{Cur: uint64(0), Max: uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_CPU, &syscall.Rlimit{Cur: uint64(traits.TimeLimitSeconds), Max: uint64(traits.TimeLimitSeconds)})
	syscall.Setrlimit(unix.RLIMIT_DATA, &syscall.Rlimit{Cur: ^uint64(0), Max: ^uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_FSIZE, &syscall.Rlimit{Cur: uint64(10 * 1024 * 1024), Max: uint64(10 * 1024 * 1024)})
	syscall.Setrlimit(unix.RLIMIT_MEMLOCK, &syscall.Rlimit{Cur: uint64(64 * 1024), Max: uint64(64 * 1024)})
	syscall.Setrlimit(unix.RLIMIT_MSGQUEUE, &syscall.Rlimit{Cur: uint64(800 * 1024), Max: uint64(800 * 1024)})
	syscall.Setrlimit(unix.RLIMIT_NICE, &syscall.Rlimit{Cur: uint64(0), Max: uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_NOFILE, &syscall.Rlimit{Cur: uint64(32), Max: uint64(32)})
	syscall.Setrlimit(unix.RLIMIT_NPROC, &syscall.Rlimit{Cur: uint64(100), Max: uint64(100)})
	syscall.Setrlimit(unix.RLIMIT_RSS, &syscall.Rlimit{Cur: ^uint64(0), Max: ^uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_RTPRIO, &syscall.Rlimit{Cur: uint64(0), Max: uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_RTTIME, &syscall.Rlimit{Cur: ^uint64(0), Max: ^uint64(0)})
	syscall.Setrlimit(unix.RLIMIT_STACK, &syscall.Rlimit{Cur: uint64(8 * 1024 * 1024), Max: uint64(8 * 1024 * 1024)})
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

func setCgroupValue(cgroupPath string, k string, v string) error {
	glog.Infof("Setting %s/%s = %s", cgroupPath, k, v)
	return ioutil.WriteFile(filepath.Join(cgroupPath, k), []byte(v), 0644)
}

func applyMemoryCgroup(cgroupPaths map[string]string, traits *accountspb.Traits, currentCgroup string) error {
	cgroupPath, err := makeCgroup("memory", currentCgroup)
	if err != nil {
		return err
	}

	if err := setCgroupValue(cgroupPath, "memory.limit_in_bytes", strconv.FormatInt(traits.MemoryLimit, 10)); err != nil {
		return err
	}

	cgroupPaths["memory"] = cgroupPath
	return nil
}

func applyCpuCgroup(cgroupPaths map[string]string, traits *accountspb.Traits, currentCgroup string) error {
	cgroupPath, err := makeCgroup("cpu", currentCgroup)
	if err != nil {
		return err
	}

	if err := setCgroupValue(cgroupPath, "cpu.shares", strconv.FormatInt(traits.CpuShares, 10)); err != nil {
		return err
	}

	cgroupPaths["cpu"] = cgroupPath
	return nil
}

func applyBlkioCgroup(cgroupPaths map[string]string, traits *accountspb.Traits, currentCgroup string) error {
	cgroupPath, err := makeCgroup("blkio", currentCgroup)
	if err != nil {
		return err
	}

	if err := setCgroupValue(cgroupPath, "blkio.weight", strconv.FormatInt(traits.BlkioWeight, 10)); err != nil {
		return err
	}

	cgroupPaths["blkio"] = cgroupPath
	return nil
}

func applyCgroups(traits *accountspb.Traits, currentCgroup string) error {
	cgroupPaths := make(map[string]string, 0)

	if err := applyMemoryCgroup(cgroupPaths, traits, currentCgroup); err != nil {
		return err
	}

	if err := applyCpuCgroup(cgroupPaths, traits, currentCgroup); err != nil {
		return err
	}

	if err := applyBlkioCgroup(cgroupPaths, traits, currentCgroup); err != nil {
		return err
	}

	if err := cgroups.EnterPid(cgroupPaths, os.Getpid()); err != nil {
		return err
	}

	return nil
}

func applyRestrictions(traits *accountspb.Traits, currentCgroup string) error {
	applyRlimits(traits)

	if err := applyCgroups(traits, currentCgroup); err != nil {
		return err
	}

	return nil
}

func main() {
	flag.Parse()

	glog.Infof("Hello! I'm a supervisor and my parent cgroup is %s!", *parentCgroup)

	ctx := context.Background()

	childStdin := os.NewFile(3, "child stdin")
	childStdout := os.NewFile(4, "child stdout")
	childStderr := os.NewFile(5, "child stderr")
	childStatus := os.NewFile(6, "child status")
	supervisorRequestFile := os.NewFile(7, "supervisor execution request")

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

	glog.Infof("Execution request from executor: %s", req)

	if !nameRegexp.MatchString(req.OwnerName) || !nameRegexp.MatchString(req.Name) {
		glog.Error("invalid owner or script name")
		os.Exit(1)
	}

	if filepath.Dir(filepath.Join(scriptMountDir, req.Name)) != scriptMountDir {
		glog.Error("invalid path")
		os.Exit(1)
	}

	executorConn, err := grpc.Dial(req.Config.ExecutorTarget, grpc.WithInsecure(), grpc.WithDialer(func(address string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", address, timeout)
	}))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	accountsClient := accountspb.NewAccountsClient(executorConn)

	glog.Infof("Retrieving account information from executor: %s", req.OwnerName)
	accountResp, err := accountsClient.Get(ctx, &accountspb.GetRequest{
		Username: req.OwnerName,
	})
	if err != nil {
		glog.Error(err)
	}
	glog.Infof("Owner account traits: %s", accountResp)

	traits := accountResp.Traits
	currentCgroup := filepath.Join(*parentCgroup, strconv.Itoa(os.Getpid()))

	if err := applyRestrictions(traits, currentCgroup); err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	factory, err := libcontainer.New(req.Config.ContainersPath, libcontainer.RootlessCgroups, libcontainer.InitArgs(req.Config.NsenternetPath, os.Args[0], "init"))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	rootfsPath := filepath.Join(req.Config.RootfsesPath, strconv.Itoa(os.Getpid()))
	if err := os.Mkdir(rootfsPath, 0755); err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	defer os.RemoveAll(rootfsPath)

	config := &configs.Config{
		Rootfs:            rootfsPath,
		Rootless:          true,
		Readonlyfs:        true,
		NoNewPrivileges:   true,
		ParentDeathSignal: int(syscall.SIGKILL),
		Capabilities: &configs.Capabilities{
			Bounding:    []string{},
			Effective:   []string{},
			Inheritable: []string{},
			Permitted:   []string{},
			Ambient:     []string{},
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
				Flags:       unix.MS_NOSUID | unix.MS_NOEXEC | unix.MS_STRICTATIME | unix.MS_RDONLY,
			},
			{
				Device:      "bind",
				Source:      filepath.Join(req.Config.StorageRootPath, req.OwnerName, "private"),
				Destination: privateMountDir,
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC,
			},
			{
				Device:      "bind",
				Source:      filepath.Join(req.Config.StorageRootPath, req.OwnerName, "private"),
				Destination: legacyPrivateMountDir,
				Flags:       unix.MS_NOSUID | unix.MS_NODEV | unix.MS_BIND | unix.MS_REC,
			},
			{
				Device:      "bind",
				Source:      filepath.Join(req.Config.StorageRootPath, req.OwnerName, "scripts"),
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
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []configs.IDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
		Seccomp: seccomp.Config,
	}

	if !traits.AllowNetworkAccess {
		config.Namespaces = append(config.Namespaces, configs.Namespace{Type: configs.NEWNET})
		config.Networks = []*configs.Network{
			{
				Type:    "loopback",
				Address: "127.0.0.1/0",
				Gateway: "localhost",
			},
		}
	}

	if traits.TmpfsSize > 0 {
		config.Mounts = append(config.Mounts, &configs.Mount{
			Device:      "tmpfs",
			Source:      "tmpfs",
			Destination: "/tmp",
			Flags:       unix.MS_NOSUID | unix.MS_NODEV,
			Data:        fmt.Sprintf("size=%d", traits.TmpfsSize),
		})
	}

	container, err := factory.Create(strconv.Itoa(os.Getpid()), config)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	defer container.Destroy()

	bridgeConn, err := grpc.Dial(req.Config.BridgeTarget, grpc.WithInsecure(), grpc.WithDialer(func(address string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", address, timeout)
	}))
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	parentFile := os.NewFile(uintptr(fds[0]), "")
	childFile := os.NewFile(uintptr(fds[1]), "")
	defer parentFile.Close()
	defer childFile.Close()

	rpcServer := rpc.NewServer()

	outputParams := &scriptspb.OutputParams{
		Format: "text",
	}
	outputService := outputservice.New(traits, outputParams)
	rpcServer.RegisterName("Output", outputService)

	params := serviceParams{
		bridgeConn:   bridgeConn,
		executorConn: executorConn,

		currentCgroup: currentCgroup,
		req:           req,
	}

	for _, serviceName := range traits.AllowedService {
		factory, ok := serviceFactories[serviceName]
		if !ok {
			glog.Warningf("Unknown service name: %s", serviceName)
			continue
		}

		service, err := factory(ctx, traits, params)
		if err != nil {
			glog.Fatalf("Failed to create service: %v", err)
		}

		rpcServer.RegisterName(serviceName, service)
	}

	serverCodec, err := srpc.NewServerCodec(parentFile)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	go rpcServer.ServeCodec(serverCodec)

	jsonK4Context, err := marshaler.MarshalToString(req.Context)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	process := &libcontainer.Process{
		Args: []string{
			"/bin/sh", "-c", shellquote.Join("exec", filepath.Join(scriptMountDir, req.Name)),
		},
		Env: []string{
			fmt.Sprintf("K4_CONTEXT=%s", jsonK4Context),
		},
		Cwd:    privateMountDir,
		Stdin:  childStdin,
		Stdout: childStdout,
		Stderr: childStderr,
		ExtraFiles: []*os.File{
			childFile,
		},
	}

	startTime := time.Now()

	if err := container.Run(process); err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	childFile.Close()

	done := make(chan struct{})
	timeLimitExceeded := false

	go func() {
		select {
		case <-time.After(time.Duration(traits.TimeLimitSeconds) * time.Second):
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

	childStdin.Close()
	childStdout.Close()
	childStderr.Close()

	endTime := time.Now()

	waitStatus := state.Sys().(syscall.WaitStatus)
	result := &scriptspb.WorkerExecutionResult{
		WaitStatus:        uint32(waitStatus),
		TimeLimitExceeded: timeLimitExceeded,
		OutputParams:      outputParams,
		Timings: &scriptspb.WorkerExecutionResult_Timings{
			RealNanos:   uint64((endTime.Sub(startTime)) / time.Nanosecond),
			UserNanos:   uint64(state.UserTime() / time.Nanosecond),
			SystemNanos: uint64(state.SystemTime() / time.Nanosecond),
		},
	}

	glog.Infof("Result: %s", result)

	raw, err := proto.Marshal(result)
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
