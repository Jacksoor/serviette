package main

import (
	"database/sql"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/trace"
	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	_ "github.com/mattn/go-sqlite3"

	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/worker"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"

	"github.com/porpoises/kobun4/executor/scriptsservice"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	socketPath      = flag.String("socket_path", "/tmp/kobun4-executor.socket", "Bind path for socket")
	debugSocketPath = flag.String("debug_socket_path", "/tmp/kobun4-executor.debug.socket", "Bind path for socket")

	sqliteDBPath = flag.String("sqlite_db_path", "executor.db", "Path to SQLite database")

	bankTarget = flag.String("bank_target", "/tmp/kobun4-bank.socket", "Bank target")

	k4LibraryPath = flag.String("k4_library_path", "clients", "Path to library root")

	scriptsRootPath = flag.String("scripts_root_path", "scripts", "Path to script root")

	imagesRootPath = flag.String("images_root_path", "images", "Path to image root")
	imageSize      = flag.Int64("image_size", 20*1024*1024, "Image size for new images")

	timeLimit   = flag.Duration("time_limit", 5*time.Second, "Time limit")
	memoryLimit = flag.Int64("memory_limit", 20*1024*1024, "Memory limit")

	durationPerUnitCost = flag.Duration("duration_per_unit_cost", time.Second, "Duration per unit cost")
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

	db, err := sql.Open("sqlite3", *sqliteDBPath)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to bank: %v", err)
	}
	defer bankConn.Close()

	supervisor := worker.NewSupervisor(&worker.WorkerOptions{
		K4LibraryPath: *k4LibraryPath,
		TimeLimit:     *timeLimit,
		MemoryLimit:   *memoryLimit,
	})

	scriptsService, err := scriptsservice.New(scripts.NewStore(*scriptsRootPath, db), *imagesRootPath, *imageSize, moneypb.NewMoneyClient(bankConn), accountspb.NewAccountsClient(bankConn), *durationPerUnitCost, supervisor)
	if err != nil {
		glog.Fatalf("could not create scripts service: %v", err)
	}
	defer scriptsService.Stop()

	s := grpc.NewServer()
	scriptspb.RegisterScriptsServer(s, scriptsService)
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
