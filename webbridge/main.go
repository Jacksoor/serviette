package main

import (
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/trace"
	_ "net/http/pprof"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/porpoises/kobun4/webbridge/handler"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket      = flag.String("bind_socket", "localhost:5904", "Bind for socket")
	bindDebugSocket = flag.String("bind_debug_socket", "localhost:5914", "Bind for socket")

	bankTarget     = flag.String("bank_target", "localhost:5901", "Bank target")
	executorTarget = flag.String("executor_target", "localhost:5902", "Executor target")

	staticPath   = flag.String("static_path", "Path to templates", "static")
	templatePath = flag.String("template_path", "Path to templates", "templates")
)

func main() {
	flag.Parse()

	grpc.EnableTracing = true
	trace.AuthRequest = func(req *http.Request) (bool, bool) {
		return true, true
	}

	debugLis, err := net.Listen("tcp", *bindDebugSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer debugLis.Close()
	glog.Infof("Debug listening on: %s", debugLis.Addr())

	go http.Serve(debugLis, nil)

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure())
	if err != nil {
		glog.Fatalf("did not connect to bank: %v", err)
	}
	defer bankConn.Close()

	executorConn, err := grpc.Dial(*executorTarget, grpc.WithInsecure())
	if err != nil {
		glog.Fatalf("did not connect to executor: %v", err)
	}
	defer executorConn.Close()

	handler, err := handler.New(*staticPath, *templatePath, accountspb.NewAccountsClient(bankConn), moneypb.NewMoneyClient(bankConn), scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to create handler: %v", err)
	}
	httpServer := &http.Server{
		Handler: handler,
	}

	lis, err := net.Listen("tcp", *bindSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
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
