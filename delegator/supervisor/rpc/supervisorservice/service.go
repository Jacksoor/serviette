package supervisorservice

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"

	srpc "github.com/porpoises/kobun4/delegator/supervisor/rpc"
)

type Child struct {
	cmd          *exec.Cmd
	wg           sync.WaitGroup
	statusReader *os.File
	statusBuf    bytes.Buffer
}

type Service struct {
	ctx context.Context

	children []*Child

	currentCgroup    string
	currentOwnerName string

	scriptsClient scriptspb.ScriptsClient

	config  *scriptspb.WorkerExecutionRequest_Configuration
	context *scriptspb.Context
}

func New(ctx context.Context, currentCgroup string, currentOwnerName string, scriptsClient scriptspb.ScriptsClient, config *scriptspb.WorkerExecutionRequest_Configuration, context *scriptspb.Context) *Service {
	return &Service{
		ctx: ctx,

		children: make([]*Child, 0),

		currentCgroup:    currentCgroup,
		currentOwnerName: currentOwnerName,

		scriptsClient: scriptsClient,

		config:  config,
		context: context,
	}
}

func (s *Service) Spawn(req *struct {
	OwnerName string `json:"ownerName"`
	Name      string `json:"name"`

	UnixRights []*os.File `json:"-"`
}, resp *srpc.Response) error {
	if len(req.UnixRights) != 3 {
		return fmt.Errorf("incorrect number of files passed: len(fds) = %d", len(req.UnixRights))
	}

	metaResp, err := s.scriptsClient.GetMeta(s.ctx, &scriptspb.GetMetaRequest{
		OwnerName: req.OwnerName,
		Name:      req.Name,
	})
	if err != nil {
		switch grpc.Code(err) {
		case codes.NotFound, codes.InvalidArgument:
			return errors.New("script not found")
		default:
			return err
		}
	}

	if metaResp.Meta.Visibility == scriptspb.Visibility_UNPUBLISHED {
		return errors.New("script not found")
	}

	glog.Infof("Supervisor is spawning: %s/%s", req.OwnerName, req.Name)

	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer statusWriter.Close()

	child := &Child{
		statusReader: statusReader,
	}

	child.wg.Add(1)
	go func() {
		io.Copy(&child.statusBuf, statusReader)
		statusReader.Close()
		child.wg.Done()
	}()

	reqReader, reqWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reqWriter.Close()
	defer reqReader.Close()

	child.cmd = exec.Command(os.Args[0], "-logtostderr", "-parent_cgroup", s.currentCgroup)
	child.cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	child.cmd.Stdout = os.Stdout
	child.cmd.Stderr = os.Stderr
	child.cmd.ExtraFiles = []*os.File{
		req.UnixRights[0],
		req.UnixRights[1],
		req.UnixRights[2],
		statusWriter,
		reqReader,
	}
	if err := child.cmd.Start(); err != nil {
		return err
	}

	req.UnixRights[0].Close()
	req.UnixRights[1].Close()
	req.UnixRights[2].Close()
	statusWriter.Close()
	reqReader.Close()

	workerReq := &scriptspb.WorkerExecutionRequest{
		Config:    s.config,
		OwnerName: req.OwnerName,
		Name:      req.Name,
		Context:   s.context,
	}

	rawReq, err := proto.Marshal(workerReq)
	if err != nil {
		return err
	}

	if _, err := reqWriter.Write(rawReq); err != nil {
		return err
	}
	reqWriter.Close()

	i := len(s.children)
	s.children = append(s.children, child)

	resp.Body = struct {
		Handle int `json:"handle"`
	}{
		i,
	}
	return nil
}

func (s *Service) Wait(req *struct {
	Handle int `json:"handle"`
}, resp *srpc.Response) error {
	if req.Handle >= len(s.children) {
		return errors.New("invalid handle")
	}
	child := s.children[req.Handle]

	if err := child.cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return err
		}
	}

	child.statusReader.Close()

	child.wg.Wait()
	rawStatus := child.statusBuf.Bytes()
	if len(rawStatus) == 0 {
		return errors.New("child never returned status")
	}

	result := &scriptspb.WorkerExecutionResult{}
	if err := proto.Unmarshal(rawStatus, result); err != nil {
		return err
	}

	resp.Body = struct {
		WaitStatus        uint32 `json:"waitStatus"`
		TimeLimitExceeded bool   `json:"timeLimitExceeded"`
		OutputFormat      string `json:"outputFormat"`
		Private           bool   `json:"private"`
	}{
		result.WaitStatus,
		result.TimeLimitExceeded,
		result.OutputParams.Format,
		result.OutputParams.Private,
	}
	return nil
}

func (s *Service) Signal(req *struct {
	Handle int `json:"handle"`
	Signal int `json:"signal"`
}, resp *srpc.Response) error {
	if req.Handle >= len(s.children) {
		return errors.New("invalid handle")
	}
	child := s.children[req.Handle]
	child.cmd.Process.Signal(syscall.Signal(req.Signal))
	return nil
}
