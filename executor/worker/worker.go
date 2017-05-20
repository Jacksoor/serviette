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
)

type Worker struct {
	opts *WorkerOptions

	arg0  string
	argv  []string
	stdin *bytes.Buffer

	rpcServer *rpc.Server
}

type WorkerOptions struct {
	NsjailPath    string
	K4LibraryPath string
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
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(
		ctx, f.opts.NsjailPath,
		append(append(nsjailArgs,
			"--mode", "o",
			"--quiet",
			"--log", "/dev/null",
			"--pass_fd", "3",
			"--user", "nobody",
			"--group", "nogroup",
			"--disable_clone_newnet",
			"--bindmount_ro", fmt.Sprintf("%s:/opt/k4", f.opts.K4LibraryPath),
			"--bindmount_ro", fmt.Sprintf("%s:/opt/k4/_work", f.arg0),
			"--bindmount_ro", "/dev/urandom",
			"--bindmount_ro", "/bin",
			"--bindmount_ro", "/sbin",
			"--bindmount_ro", "/etc/ssl/certs",
			"--bindmount_ro", "/etc/resolv.conf",
			"--bindmount_ro", "/usr",
			"--bindmount_ro", "/lib",
			"--bindmount_ro", "/lib64",
			"--",
			"/opt/k4/_work"),
			f.argv...)...)
	cmd.Stdin = f.stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	childFile := os.NewFile(uintptr(fds[1]), "")
	cmd.ExtraFiles = []*os.File{childFile}

	parentFile := os.NewFile(uintptr(fds[0]), "")
	defer parentFile.Close()

	go f.rpcServer.ServeCodec(jsonrpc.NewServerCodec(parentFile))

	if err := cmd.Start(); err != nil {
		childFile.Close()
		return nil, err
	}
	childFile.Close()

	if err := cmd.Wait(); err != nil {
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
