package main

import (
	"flag"
	"net"
	"os"
	"path/filepath"

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
	bind       = flag.String("bind", "localhost:50052", "Bind host")
	bankTarget = flag.String("bank_target", "localhost:50051", "Bank target")
	nsjailPath = flag.String("nsjail_path", "nsjail", "Path to nsjail")
)

func main() {
	flag.Parse()

	lis, err := net.Listen("tcp", *bind)
	glog.Infof("Listening on: %s", lis.Addr())

	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		glog.Fatalf("failed to get working dir: %v", err)
	}
	k4LibraryPath := filepath.Join(wd, "clients")

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure())
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

	if err := s.Serve(lis); err != nil {
		glog.Fatalf("failed to serve: %v", err)
	}
}
