package worker

import (
	"bytes"
	"context"
	"fmt"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/djherbis/buffer/limio"
)

const maxBufferSize int64 = 5 * 1024 * 1024 // 5MB

type Worker struct {
	opts *WorkerOptions

	arg0  string
	argv  []string
	stdin *bytes.Buffer

	rpcServer *rpc.Server
}

type WorkerOptions struct {
	TimeLimit          time.Duration
	MemoryLimit        int64
	TmpfsSize          int64
	Chroot             string
	KafelSeccompPolicy string

	Network *NetworkOptions
}

type NetworkOptions struct {
	Interface string
	IP        string
	Netmask   string
	Gateway   string
}

type WorkerResult struct {
	Stdout       []byte
	Stderr       []byte
	ProcessState *os.ProcessState
}

func newWorker(opts *WorkerOptions, arg0 string, argv []string, stdin []byte) *Worker {
	return &Worker{
		opts: opts,

		arg0:  arg0,
		argv:  argv,
		stdin: bytes.NewBuffer(stdin),

		rpcServer: rpc.NewServer(),
	}
}

func (f *Worker) Run(ctx context.Context, nsjailArgs []string) (*WorkerResult, error) {
	ctx, cancel := context.WithTimeout(ctx, f.opts.TimeLimit)
	defer cancel()

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if deadline, ok := ctx.Deadline(); ok {
		timeLimit := time.Until(deadline) / time.Second
		nsjailArgs = append(nsjailArgs,
			"--rlimit_cpu", fmt.Sprintf("%d", timeLimit),
			"--time_limit", fmt.Sprintf("%d", timeLimit),
		)
	}

	nsjailArgs = append(nsjailArgs,
		"--mode", "o",
		"--log", "/proc/self/fd/4",
		"--pass_fd", "3",
		"--user", "nobody",
		"--group", "nogroup",
		"--hostname", "kobun4",
		"--cgroup_mem_max", fmt.Sprintf("%d", f.opts.MemoryLimit),
		"--chroot", f.opts.Chroot,
		"--tmpfsmount", "/tmp",
		"--tmpfs_size", fmt.Sprintf("%d", f.opts.TmpfsSize),
		"--seccomp_string", f.opts.KafelSeccompPolicy,
		"--macvlan_iface", f.opts.Network.Interface,
		"--macvlan_vs_ip", f.opts.Network.IP,
		"--macvlan_vs_nm", f.opts.Network.Netmask,
		"--macvlan_vs_gw", f.opts.Network.Gateway,
		"--", f.arg0)

	cmd := exec.CommandContext(
		ctx, "nsjail", append(nsjailArgs, f.argv...)...)
	cmd.Stdin = f.stdin
	cmd.Stdout = limio.LimitWriter(&stdout, maxBufferSize)
	cmd.Stderr = limio.LimitWriter(&stderr, maxBufferSize)

	childFile := os.NewFile(uintptr(fds[1]), "")
	cmd.ExtraFiles = []*os.File{
		childFile,
		os.Stderr,
	}

	parentFile := os.NewFile(uintptr(fds[0]), "")
	defer parentFile.Close()

	go f.rpcServer.ServeCodec(jsonrpc.NewServerCodec(parentFile))

	if err := cmd.Start(); err != nil {
		childFile.Close()
		return nil, err
	}
	childFile.Close()

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &WorkerResult{
				Stdout:       stdout.Bytes(),
				Stderr:       stderr.Bytes(),
				ProcessState: exitErr.ProcessState,
			}, err
		}
		return nil, err
	}

	return &WorkerResult{
		Stdout:       stdout.Bytes(),
		Stderr:       stderr.Bytes(),
		ProcessState: cmd.ProcessState,
	}, nil
}

func (f *Worker) RegisterService(name string, rcvr interface{}) error {
	return f.rpcServer.RegisterName(name, rcvr)
}
