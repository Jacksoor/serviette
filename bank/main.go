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

	_ "github.com/mattn/go-sqlite3"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	"github.com/porpoises/kobun4/bank/accounts"

	"github.com/porpoises/kobun4/bank/accountsservice"
	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	"github.com/porpoises/kobun4/bank/moneyservice"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	"github.com/porpoises/kobun4/bank/namesservice"
	namespb "github.com/porpoises/kobun4/bank/namesservice/v1pb"
)

var (
	socketPath      = flag.String("socket_path", "/tmp/kobun4-bank.socket", "Bind path for socket")
	debugSocketPath = flag.String("debug_socket_path", "/tmp/kobun4-bank.debug.socket", "Bind path for socket")
	sqliteDBPath    = flag.String("sqlite_db_path", "bank.db", "Path to SQLite database")
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

	accountsStore := accounts.NewStore(db)

	s := grpc.NewServer()
	accountspb.RegisterAccountsServer(s, accountsservice.New(accountsStore))
	namespb.RegisterNamesServer(s, namesservice.New(accountsStore))
	moneypb.RegisterMoneyServer(s, moneyservice.New(accountsStore))

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
