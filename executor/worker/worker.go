package worker

import (
	"context"
	"fmt"
	"io"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type Worker struct {
	nsjailArgs []string

	arg0 string
	argv []string

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	rpcServer *rpc.Server
}

func New(nsjailArgs []string, arg0 string, argv []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Worker {
	return &Worker{
		nsjailArgs: nsjailArgs,

		arg0: arg0,
		argv: argv,

		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,

		rpcServer: rpc.NewServer(),
	}
}

func (w *Worker) Run(ctx context.Context) (*os.ProcessState, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	nsjailArgs := append([]string{
		"--mode", "o",
		"--log_fd", "4",
		"--pass_fd", "3",
		"--enable_clone_newcgroup",
	}, w.nsjailArgs...)

	if deadline, ok := ctx.Deadline(); ok {
		timeLimit := time.Until(deadline)
		nsjailArgs = append(nsjailArgs,
			"--rlimit_cpu", fmt.Sprintf("%d", timeLimit/time.Second),
			"--time_limit", fmt.Sprintf("%d", timeLimit/time.Second),
		)
	}

	cmd := exec.CommandContext(ctx, "nsjail", append(append(nsjailArgs, "--", w.arg0), w.argv...)...)

	cmd.Stdin = w.stdin
	cmd.Stdout = w.stdout
	cmd.Stderr = w.stderr

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
			return exitErr.ProcessState, err
		}
		return nil, err
	}

	return cmd.ProcessState, nil
}

func (w *Worker) RegisterService(name string, rcvr interface{}) error {
	return w.rpcServer.RegisterName(name, rcvr)
}
