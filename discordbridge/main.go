package main

import (
	"encoding/json"
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

	"github.com/porpoises/kobun4/discordbridge/client"
	"github.com/porpoises/kobun4/discordbridge/networkinfoservice"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket      = flag.String("bind_socket", "localhost:5903", "Bind for socket")
	bindDebugSocket = flag.String("bind_debug_socket", "localhost:5913", "Bind for socket")

	discordToken = flag.String("discord_token", "", "Token for Discord.")
	status       = flag.String("status", "", "Status to show.")

	flavors = flag.String("flavors", `{"": {"bankCommandPrefix": "$", "scriptCommandPrefix": "!", "currencyName": "coins", "quiet": false}}`, "Per-guild flavors")

	bankCommandPrefix   = flag.String("bank_command_prefix", "$", "Bank command prefix")
	scriptCommandPrefix = flag.String("script_command_prefix", "!", "Script command prefix")
	currencyName        = flag.String("currency_name", "coins", "Currency name")

	bankTarget     = flag.String("bank_target", "localhost:5901", "Bank target")
	executorTarget = flag.String("executor_target", "localhost:5902", "Executor target")

	paymentPerMessageCharacter = flag.Int64("payment_per_message_character", 1, "How much to pay per character in a message")

	webURL = flag.String("web_url", "http://kobun", "URL to web UI")
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

	var flavorsMap map[string]client.Flavor
	if err := json.Unmarshal([]byte(*flavors), &flavorsMap); err != nil {
		glog.Fatalf("did not load flavors: %v", err)
	}

	client, err := client.New(*discordToken, &client.Options{
		Status:  *status,
		Flavors: flavorsMap,
		WebURL:  *webURL,
	}, *bindSocket, *paymentPerMessageCharacter, accountspb.NewAccountsClient(bankConn), moneypb.NewMoneyClient(bankConn), scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to connect to discord: %v", err)
	}
	defer client.Close()

	glog.Info("Connected to Discord.")

	s := grpc.NewServer()
	networkinfopb.RegisterNetworkInfoServer(s, networkinfoservice.New(client.Session()))
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

	select {
	case err := <-errChan:
		glog.Fatalf("failed to serve: %v", err)
	case s := <-signalChan:
		glog.Infof("Got signal: %s", s)
	}
}
