package supervisor

import (
	"bytes"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"os/exec"
	"syscall"
)

type Follower struct {
	*os.File
	*exec.Cmd
	rpcServer *rpc.Server
}

func NewFollower(nsjailPath string, arg0 string, argv []string, stdin []byte) (*Follower, error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	var cmd *exec.Cmd
	if nsjailPath != "" {
		cmd = exec.Command(
			nsjailPath,
			append([]string{
				"--mode", "o",
				"--bindmount_ro", "/dev/urandom",
				"--bindmount_ro", "/bin",
				"--bindmount_ro", "/sbin",
				"--bindmount_ro", "/etc/ssl/certs",
				"--bindmount_ro", "/etc/resolv.conf",
				"--bindmount_ro", "/usr",
				"--bindmount_ro", "/lib",
				"--bindmount_ro", "/lib64",
				"--user", "nobody",
				"--group", "nogroup",
				"--disable_clone_newnet",
				"--",
				arg0,
			}, argv...)...)
	} else {
		cmd = exec.Command(arg0, argv...)
	}
	cmd.Stdin = bytes.NewBuffer(stdin)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{os.NewFile(uintptr(fds[1]), "")}

	rpcServer := rpc.NewServer()

	file := os.NewFile(uintptr(fds[0]), "")
	go rpcServer.ServeCodec(jsonrpc.NewServerCodec(file))

	return &Follower{
		File:      file,
		Cmd:       cmd,
		rpcServer: rpcServer,
	}, nil
}

func (f *Follower) RegisterService(name string, rcvr interface{}) error {
	return f.rpcServer.RegisterName(name, rcvr)
}
