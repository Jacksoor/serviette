package main

import (
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/trace"
	_ "net/http/pprof"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/porpoises/kobun4/webbridge/handler"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	deedspb "github.com/porpoises/kobun4/bank/deedsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	socketPath      = flag.String("socket_path", "/tmp/kobun4-webbridge.socket", "Bind path for socket")
	debugSocketPath = flag.String("debug_socket_path", "/tmp/kobun4-discordbridge.debug.socket", "Bind path for socket")

	bankTarget     = flag.String("bank_target", "/tmp/kobun4-bank.socket", "Bank target")
	executorTarget = flag.String("executor_target", "/tmp/kobun4-executor.socket", "Executor target")

	staticPath   = flag.String("static_path", "Path to templates", "static")
	templatePath = flag.String("template_path", "Path to templates", "templates")
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

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to bank: %v", err)
	}
	defer bankConn.Close()

	executorConn, err := grpc.Dial(*executorTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to executor: %v", err)
	}
	defer executorConn.Close()

	handler, err := handler.New(*staticPath, *templatePath, accountspb.NewAccountsClient(bankConn), deedspb.NewDeedsClient(bankConn), moneypb.NewMoneyClient(bankConn), scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to create handler: %v", err)
	}
	httpServer := &http.Server{
		Handler: handler,
	}

	lis, err := net.Listen("unix", *socketPath)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	if err := os.Chmod(*socketPath, 0777); err != nil {
		glog.Fatalf("failed to chmod listener: %v", err)
	}

	glog.Infof("Listening on: %v", lis.Addr())

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	errChan := make(chan error)
	go func() {
		errChan <- httpServer.Serve(lis)
	}()

	select {
	case err := <-errChan:
		glog.Fatalf("failed to serve: %v", err)
	case s := <-signalChan:
		glog.Infof("Got signal: %s", s)
	}
}
