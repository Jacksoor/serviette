package main

import (
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/net/trace"
	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	"github.com/porpoises/kobun4/executor/pricing"
	"github.com/porpoises/kobun4/executor/worker"

	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/scriptsservice"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	socketPath      = flag.String("socket_path", "/tmp/kobun4-executor.socket", "Bind path for socket")
	debugSocketPath = flag.String("debug_socket_path", "/tmp/kobun4-executor.debug.socket", "Bind path for socket")
	bankTarget      = flag.String("bank_target", "/tmp/kobun4-bank.socket", "Bank target")
	nsjailPath      = flag.String("nsjail_path", "nsjail", "Path to nsjail")
)

func main() {
	flag.Parse()

	grpc.EnableTracing = true
	trace.AuthRequest = func(req *http.Request) (bool, bool) {
		return true, true
	}

	debugLis, err := net.Listen("unix", *debugSocketPath)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer debugLis.Close()
	glog.Infof("Debug listening on: %s", debugLis.Addr())

	go http.Serve(debugLis, nil)

	wd, err := os.Getwd()
	if err != nil {
		glog.Fatalf("failed to get working dir: %v", err)
	}
	k4LibraryPath := filepath.Join(wd, "clients")

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to bank: %v", err)
	}
	defer bankConn.Close()

	pricer := &pricing.FactorPricer{
		CPUTimeNum: 1,
		CPUTimeDen: 1000,

		MemoryNum: 1,
		MemoryDen: 1000000,
	}

	supervisor := worker.NewSupervisor(&worker.WorkerOptions{
		NsjailPath:    *nsjailPath,
		K4LibraryPath: k4LibraryPath,
	})

	s := grpc.NewServer()
	scriptspb.RegisterScriptsServer(s, scriptsservice.New(moneypb.NewMoneyClient(bankConn), assetspb.NewAssetsClient(bankConn), pricer, supervisor))
	reflection.Register(s)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	lis, err := net.Listen("unix", *socketPath)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	glog.Infof("Listening on: %s", lis.Addr())

	errChan := make(chan error)
	go func() {
		errChan <- s.Serve(lis)
	}()

	select {
	case err := <-errChan:
		glog.Fatalf("failed to serve: %v", err)
	case s := <-signalChan:
		glog.Infof("Got signal: %s", s)
	}
}
