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

	"github.com/emicklei/go-restful"

	"github.com/porpoises/kobun4/restbridge/rest"

	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket      = flag.String("bind_socket", "localhost:5905", "Bind for socket")
	bindDebugSocket = flag.String("bind_debug_socket", "localhost:5915", "Bind for socket")

	tokenSecret   = flag.String("token_secret", "", "Token secret")
	tokenDuration = flag.Duration("token_duration", 24*time.Hour, "Token duration")

	executorTarget = flag.String("executor_target", "localhost:5902", "Executor target")
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

	if *tokenSecret == "" {
		glog.Fatal("-token_secret not provided")
	}

	executorConn, err := grpc.Dial(*executorTarget, grpc.WithInsecure())
	if err != nil {
		glog.Fatalf("did not connect to executor: %v", err)
	}
	defer executorConn.Close()

	wsContainer := restful.NewContainer()

	accountsClient := accountspb.NewAccountsClient(executorConn)
	scriptsClient := scriptspb.NewScriptsClient(executorConn)

	secret := []byte(*tokenSecret)

	accountsResource := rest.NewAccountsResource(secret, accountsClient, scriptsClient)
	loginResource := rest.NewLoginResource(secret, *tokenDuration, accountsClient)

	wsContainer.Add(accountsResource.WebService())
	wsContainer.Add(loginResource.WebService())

	httpServer := &http.Server{
		Handler: wsContainer,
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
