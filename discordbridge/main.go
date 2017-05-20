package main

import (
	"database/sql"
	"flag"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/porpoises/kobun4/discordbridge/client"
	"github.com/porpoises/kobun4/discordbridge/store"

	accountspb "github.com/porpoises/kobun4/bank/accountsservice/v1pb"
	assetspb "github.com/porpoises/kobun4/bank/assetsservice/v1pb"
	moneypb "github.com/porpoises/kobun4/bank/moneyservice/v1pb"
	scriptspb "github.com/porpoises/kobun4/executor/scriptsservice/v1pb"
)

var (
	discordToken = flag.String("discord_token", "", "Token for Discord.")
	status       = flag.String("status", "", "Status to show.")

	bankCommandPrefix   = flag.String("bank_command_prefix", "$", "Bank command prefix")
	scriptCommandPrefix = flag.String("script_command_prefix", "!", "Script command prefix")
	currencyName        = flag.String("currency_name", "coins", "Currency name")

	sqliteDBPath = flag.String("sqlite_db_path", "discordbridge.db", "SQLite database path")

	bankTarget     = flag.String("bank_target", "/tmp/kobun4-bank.socket", "Bank target")
	executorTarget = flag.String("executor_target", "/tmp/kobun4-executor.socket", "Executor target")
)

func main() {
	flag.Parse()

	db, err := sql.Open("sqlite3", *sqliteDBPath)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}
	store := store.New(db)

	bankConn, err := grpc.Dial(*bankTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to bank: %v", err)
	}
	defer bankConn.Close()

	executorConn, err := grpc.Dial(*executorTarget, grpc.WithInsecure(), grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
		return net.DialTimeout("unix", addr, timeout)
	}))
	if err != nil {
		glog.Fatalf("did not connect to executor: %v", err)
	}
	defer executorConn.Close()

	client, err := client.New(*discordToken, &client.Options{
		Status:              *status,
		BankCommandPrefix:   *bankCommandPrefix,
		ScriptCommandPrefix: *scriptCommandPrefix,
		CurrencyName:        *currencyName,
	}, store, accountspb.NewAccountsClient(bankConn), assetspb.NewAssetsClient(bankConn), moneypb.NewMoneyClient(bankConn), scriptspb.NewScriptsClient(executorConn))
	if err != nil {
		glog.Fatalf("failed to connect to discord: %v", err)
	}
	defer client.Close()

	glog.Info("Connected to Discord.")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill, syscall.SIGTERM)

	s := <-signalChan
	glog.Infof("Got signal: %s", s)
}
