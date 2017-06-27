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

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket      = flag.String("bind_socket", "/run/kobun4-webbridge/main.socket", "Bind for socket")
	bindDebugSocket = flag.String("bind_debug_socket", "/run/kobun4-webbridge/debug.socket", "Bind for socket")

	executorTarget = flag.String("executor_target", "/run/kobun4-executor/main.socket", "Executor target")

	staticPath   = flag.String("static_path", "Path to templates", "static")
	templatePath = flag.String("template_path", "Path to templates", "templates")
)

func main() {
	flag.Parse()

	grpc.EnableTracing = true
	trace.AuthRequest = func(req *http.Request) (bool, bool) {
		return true, true
	}

	os.Remove(*bindDebugSocket)
	debugLis, err := net.Listen("unix", *bindDebugSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer debugLis.Close()
	glog.Infof("Debug listening on: %s", debugLis.Addr())

	go http.Serve(debugLis, nil)

	executorConn, err := grpc.Dial(*executorTarget, grpc.WithInsecure(), grpc.WithDialer(func(address string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", address, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to executor: %v", err)
	}
	defer executorConn.Close()

	handler, err := handler.New(*staticPath, *templatePath, accountspb.NewAccountsClient(executorConn), scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to create handler: %v", err)
	}
	httpServer := &http.Server{
		Handler: handler,
	}

	os.Remove(*bindSocket)
	lis, err := net.Listen("unix", *bindSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	os.Chmod(*bindSocket, 0777)
	defer lis.Close()

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
