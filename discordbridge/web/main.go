package main

import (
	"database/sql"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/porpoises/kobun4/discordbridge/web/handler"

	"github.com/golang/glog"
	_ "github.com/lib/pq"
)

var (
	bindSocket = flag.String("bind_socket", "/run/kobun4-discordbridge-web/web.socket", "Bind for socket")

	baseURL      = flag.String("base_url", "", "Base URL for discordweb.")
	clientID     = flag.String("client_id", "", "Client ID")
	clientSecret = flag.String("client_secret", "", "Client secret")
	botToken     = flag.String("bot_token", "", "Bot token")

	defaultAnnouncement = flag.String("default_announcement", "", "Default announcement")

	postgresURL = flag.String("postgres_url", "postgres://", "URL to Postgres database")
)

func main() {
	flag.Parse()

	db, err := sql.Open("postgres", *postgresURL)
	if err != nil {
		glog.Fatalf("failed to open db: %v", err)
	}

	handler, err := handler.New(*baseURL, *clientID, *clientSecret, *botToken, *defaultAnnouncement, db)
	if err != nil {
		glog.Fatalf("failed to create handler: %v", err)
	}

	httpServer := &http.Server{
		Handler: handler,
	}

	lis, err := net.Listen("unix", *bindSocket)
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
