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
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/grpclog/glogger"
	"google.golang.org/grpc/reflection"

	"github.com/porpoises/kobun4/discordbridge/budget"
	"github.com/porpoises/kobun4/discordbridge/client"

	"github.com/porpoises/kobun4/discordbridge/messagingservice"
	"github.com/porpoises/kobun4/discordbridge/networkinfoservice"
	"github.com/porpoises/kobun4/discordbridge/statsservice"

	"github.com/porpoises/kobun4/discordbridge/statsstore"
	"github.com/porpoises/kobun4/discordbridge/varstore"

	messagingpb "github.com/porpoises/kobun4/executor/messagingservice/v1pb"
	networkinfopb "github.com/porpoises/kobun4/executor/networkinfoservice/v1pb"
	statspb "github.com/porpoises/kobun4/executor/statsservice/v1pb"

	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	bindSocket      = flag.String("bind_socket", "/run/kobun4-discordbridge/main.socket", "Bind for socket")
	bindDebugSocket = flag.String("bind_debug_socket", "/run/kobun4-discordbridge/debug.socket", "Bind for socket")

	botToken = flag.String("bot_token", "", "Bot token.")
	status   = flag.String("status", "", "Status to show.")

	discordBotsToken = flag.String("discord_bots_token", "", "Token for posting stats to bots.discord.pw")

	knownGuildsOnly = flag.Bool("known_guilds_only", false, "Only stay on known guilds")

	postgresURL = flag.String("postgres_url", "postgres://", "URL to Postgres database")

	executorTarget = flag.String("executor_target", "/run/kobun4-executor/main.socket", "Executor target")

	homeURL = flag.String("home_url", "http://kobun", "URL to web UI")

	maxBudgetPerUser    = flag.Duration("max_budget_per_user", 1*time.Second, "Max execution budget per user")
	payoutPeriodPerUser = flag.Duration("payout_period_per_user", 10, "How much time to wait before paying 1 unit of time back")
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

	db, err := sql.Open("postgres", *postgresURL)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	vars := varstore.New(db)
	stats := statsstore.New(db)

	os.Remove(*bindSocket)
	lis, err := net.Listen("unix", *bindSocket)
	if err != nil {
		glog.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()
	os.Chmod(*bindSocket, 0777)
	glog.Infof("Listening on: %s", lis.Addr())

	budgeter := budget.New(db, *maxBudgetPerUser, *payoutPeriodPerUser)

	client, err := client.New(*botToken, &client.Options{
		Status:  *status,
		HomeURL: *homeURL,
	}, *discordBotsToken, *knownGuildsOnly, lis.Addr(), vars, stats, budgeter, scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to connect to discord: %v", err)
	}
	defer client.Close()

	glog.Info("Connected to Discord.")

	s := grpc.NewServer()
	networkinfopb.RegisterNetworkInfoServer(s, networkinfoservice.New(client.Session(), vars))
	messagingpb.RegisterMessagingServer(s, messagingservice.New(client.Session(), vars))
	statspb.RegisterStatsServer(s, statsservice.New(stats))
	reflection.Register(s)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

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
