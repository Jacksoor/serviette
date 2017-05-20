package main

import (
	"database/sql"
	"flag"
	"net"

	_ "github.com/mattn/go-sqlite3"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	"github.com/porpoises/kobun4/bank/accounts"

	"github.com/porpoises/kobun4/bank/accountsservice"
	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	"github.com/porpoises/kobun4/bank/assetsservice"
	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	"github.com/porpoises/kobun4/bank/moneyservice"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
)

var (
	bind         = flag.String("bind", "localhost:50051", "Bind host")
	sqliteDBPath = flag.String("sqlite_db_path", "bank.db", "Path to SQLite database")
)

func main() {
	flag.Parse()

	db, err := sql.Open("sqlite3", *sqliteDBPath)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	lis, err := net.Listen("tcp", *bind)
	glog.Infof("Listening on: %s", lis.Addr())

	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}

	accountsStore := accounts.NewStore(db)

	s := grpc.NewServer()
	accountspb.RegisterAccountsServer(s, accountsservice.New(accountsStore))
	assetspb.RegisterAssetsServer(s, assetsservice.New(accountsStore))
	moneypb.RegisterMoneyServer(s, moneyservice.New(accountsStore))

	reflection.Register(s)

	if err := s.Serve(lis); err != nil {
		glog.Fatalf("failed to serve: %v", err)
	}
}
