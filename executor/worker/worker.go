package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	opts *Options

	arg0  string
	argv  []string
	stdin io.Reader

	rpcServer *rpc.Server
}

type Options struct {
	Chroot             string
	KafelSeccompPolicy string
	Network            *NetworkOptions

	TimeLimit   time.Duration
	MemoryLimit int64
	TmpfsSize   int64

	ExtraNsjailArgs []string
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

func New(opts *Options, arg0 string, argv []string, stdin io.Reader) *Worker {
	return &Worker{
		opts: opts,

		arg0:  arg0,
		argv:  argv,
		stdin: stdin,

		rpcServer: rpc.NewServer(),
	}
}

func (w *Worker) Run(ctx context.Context) (*WorkerResult, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, w.opts.TimeLimit)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("no deadline found?")
	}

	timeLimit := time.Until(deadline)

	nsjailArgs := []string{
		"--mode", "o",
		"--log_fd", "4",
		"--pass_fd", "3",
		"--user", "nobody",
		"--group", "nogroup",
		"--hostname", "kobun4",
		"--cgroup_mem_max", fmt.Sprintf("%d", w.opts.MemoryLimit),
		"--cgroup_mem_parent", "/",
		"--cgroup_pids_parent", "/",
		"--rlimit_cpu", fmt.Sprintf("%d", timeLimit/time.Second),
		"--time_limit", fmt.Sprintf("%d", timeLimit/time.Second),
		"--chroot", w.opts.Chroot,
		"--tmpfsmount", "/tmp",
		"--tmpfs_size", fmt.Sprintf("%d", w.opts.TmpfsSize),
		"--seccomp_string", w.opts.KafelSeccompPolicy,
		"--macvlan_iface", w.opts.Network.Interface,
		"--macvlan_vs_ip", w.opts.Network.IP,
		"--macvlan_vs_nm", w.opts.Network.Netmask,
		"--macvlan_vs_gw", w.opts.Network.Gateway,
	}

	nsjailArgs = append(nsjailArgs, w.opts.ExtraNsjailArgs...)
	nsjailArgs = append(nsjailArgs, "--", w.arg0)

	cmd := exec.CommandContext(
		ctx, "nsjail", append(nsjailArgs, w.argv...)...)
	cmd.Stdin = w.stdin
	cmd.Stdout = limio.LimitWriter(&stdout, maxBufferSize)
	cmd.Stderr = limio.LimitWriter(&stderr, maxBufferSize)

	childFile := os.NewFile(uintptr(fds[1]), "")
	cmd.ExtraFiles = []*os.File{
		childFile,
		os.Stderr,
	}

	parentFile := os.NewFile(uintptr(fds[0]), "")
	defer parentFile.Close()

	go w.rpcServer.ServeCodec(jsonrpc.NewServerCodec(parentFile))

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

func (w *Worker) RegisterService(name string, rcvr interface{}) error {
	return w.rpcServer.RegisterName(name, rcvr)
}
