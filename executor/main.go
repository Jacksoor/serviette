package main

import (
	"database/sql"
	"flag"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"golang.org/x/net/trace"
	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/webdav"

	"github.com/porpoises/kobun4/executor/accountsservice"
	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	"github.com/porpoises/kobun4/executor/scriptsservice"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket       = flag.String("bind_socket", "/run/kobun4-executor/main.socket", "Bind for socket")
	bindDebugSocket  = flag.String("bind_debug_socket", "/run/kobun4-executor/debug.socket", "Bind for socket")
	bindWebdavSocket = flag.String("bind_webdav_socket", "/run/kobun4-executor/webdav.socket", "Bind for WebDAV socket")

	postgresURL = flag.String("postgres_url", "postgres://", "URL to Postgres database")

	nsenternetPath = flag.String("nsenternet_path", "executor/tools/nsenternet/nsenternet", "Path to nsenternet")
	supervisorPath = flag.String("supervisor_path", "delegator/supervisor/supervisor", "Path to supervisor")

	k4LibraryPath   = flag.String("k4_library_path", "clients", "Path to library root")
	chrootPath      = flag.String("chroot_path", "chroot", "Path to chroot")
	parentCgroup    = flag.String("parent_cgroup", "kobun4-executor", "Parent cgroup")
	storageRootPath = flag.String("storage_root_path", "storage", "Path to image root")
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

	http.Handle("/metrics", promhttp.Handler())

	go http.Serve(debugLis, nil)

	db, err := sql.Open("postgres", *postgresURL)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	storageRootAbsPath, err := filepath.Abs(*storageRootPath)
	if err != nil {
		glog.Fatalf("failed to get storage root path: %v", err)
	}

	accountStore := accounts.NewStore(db, storageRootAbsPath)
	scriptsStore := scripts.NewStore(db, storageRootAbsPath)

	os.Remove(*bindSocket)
	lis, err := net.Listen("unix", *bindSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	os.Chmod(*bindSocket, 0777)
	glog.Infof("Listening on: %s", lis.Addr())

	s := grpc.NewServer()
	scriptspb.RegisterScriptsServer(s, scriptsservice.New(lis, scriptsStore, accountStore, *nsenternetPath, *supervisorPath, *k4LibraryPath, *chrootPath, *parentCgroup))
	accountspb.RegisterAccountsServer(s, accountsservice.New(accountStore))
	reflection.Register(s)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	errChan := make(chan error)
	go func() {
		errChan <- s.Serve(lis)
	}()

	os.Remove(*bindWebdavSocket)
	webdavLis, err := net.Listen("unix", *bindWebdavSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer webdavLis.Close()
	os.Chmod(*bindWebdavSocket, 0777)
	glog.Infof("WebDAV listening on: %s", webdavLis.Addr())

	httpServer := &http.Server{
		Handler: webdav.NewHandler(accountStore),
	}
	go func() {
		errChan <- httpServer.Serve(webdavLis)
	}()

	select {
	case err := <-errChan:
		glog.Fatalf("failed to serve: %v", err)
	case s := <-signalChan:
		glog.Infof("Got signal: %s", s)
	}
}
