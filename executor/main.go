package main

import (
	"database/sql"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/net/trace"
	"net/http"
	_ "net/http/pprof"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	_ "github.com/mattn/go-sqlite3"

	"github.com/porpoises/kobun4/executor/accounts"
	"github.com/porpoises/kobun4/executor/scripts"
	"github.com/porpoises/kobun4/executor/webdav"
	"github.com/porpoises/kobun4/executor/worker"

	"github.com/porpoises/kobun4/executor/accountsservice"
	accountspb "github.com/porpoises/kobun4/executor/accountsservice/v1pb"
	"github.com/porpoises/kobun4/executor/scriptsservice"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket       = flag.String("bind_socket", "localhost:5902", "Bind for socket")
	bindDebugSocket  = flag.String("bind_debug_socket", "localhost:5912", "Bind for socket")
	bindWebdavSocket = flag.String("bind_webdav_socket", "localhost:5922", "Bind for WebDAV socket")

	sqliteDBPath = flag.String("sqlite_db_path", "executor.db", "Path to SQLite database")

	k4LibraryPath   = flag.String("k4_library_path", "clients", "Path to library root")
	chrootPath      = flag.String("chroot_path", "chroot", "Path to chroot")
	scriptsRootPath = flag.String("scripts_root_path", "scripts", "Path to script root")

	imagesRootPath = flag.String("images_root_path", "images", "Path to image root")
	imageSize      = flag.Int64("image_size", 20*1024*1024, "Image size for new images")

	kafelSeccompPolicy = flag.String("kafel_seccomp_policy", "POLICY default { KILL { ptrace, process_vm_readv, process_vm_writev } } USE default DEFAULT ALLOW", "Kafel policy to use for seccomp")

	macvlanIface = flag.String("macvlan_iface", "veth1", "Network interface which will be cloned as 'vs'")
	macvlanVsIP  = flag.String("macvlan_vs_ip", "10.0.0.2", "IP of the 'vs' interface")
	macvlanVsNM  = flag.String("macvlan_vs_nm", "255.255.255.0", "Netmask of the 'vs' interface")
	macvlanVsGW  = flag.String("macvlan_vs_gw", "10.0.0.1", "Gateway of the 'vs' interface")
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

	db, err := sql.Open("sqlite3", *sqliteDBPath)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	mounter, err := scripts.NewMounter(*imagesRootPath, *imageSize)
	if err != nil {
		glog.Fatalf("could not create scripts service: %v", err)
	}
	defer func() {
		if err := mounter.Close(); err != nil {
			glog.Errorf("failed to close mounter: %v", err)
		}
	}()

	accountStore := accounts.NewStore(db)

	scriptsStore, err := scripts.NewStore(*scriptsRootPath)
	if err != nil {
		glog.Fatalf("failed to open scripts store: %v", err)
	}

	s := grpc.NewServer()
	scriptspb.RegisterScriptsServer(s, scriptsservice.New(scriptsStore, accountStore, mounter, *k4LibraryPath, &worker.Options{
		Chroot:             *chrootPath,
		KafelSeccompPolicy: *kafelSeccompPolicy,
		Network: &worker.NetworkOptions{
			Interface: *macvlanIface,
			IP:        *macvlanVsIP,
			Netmask:   *macvlanVsNM,
			Gateway:   *macvlanVsGW,
		},
	}))
	accountspb.RegisterAccountsServer(s, accountsservice.New(accountStore))
	reflection.Register(s)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	lis, err := net.Listen("tcp", *bindSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	glog.Infof("Listening on: %s", lis.Addr())

	errChan := make(chan error)
	go func() {
		errChan <- s.Serve(lis)
	}()

	webdavLis, err := net.Listen("tcp", *bindWebdavSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer webdavLis.Close()

	glog.Infof("WebDAV listening on: %s", webdavLis.Addr())

	httpServer := &http.Server{
		Handler: webdav.NewHandler(mounter, accountStore),
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
