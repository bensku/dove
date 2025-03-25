package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/bensku/dove/admin"
	"github.com/bensku/dove/nameserver"
	"github.com/bensku/dove/zone"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func main() {
	httpListen := flag.String("admin-addr", ":8080", "Listen address for HTTP admin API")
	dnsListen := flag.String("dns-addr", ":53", "Listen address for DNS server")
	etcdEndpoints := flag.String("etcd-endpoints", "", "Comma-separated list of etcd endpoints")
	etcdPrefix := flag.String("etcd-prefix", "/dove/zones", "Etcd prefix for zone data")
	localData := flag.String("fallback-dir", "/tmp/dove/zones", "Local path for fallback zone data")
	apiKeys := flag.String("accept-keys", "", "Comma-separated list of accepted API keys for admin API")
	logLevel := flag.String("log-level", "INFO", "Log level")
	flag.Parse()

	if *etcdEndpoints == "" {
		slog.Error("--etcd-endpoints is required")
		return
	}

	// Setup logging
	var programLevel = new(slog.LevelVar)
	programLevel.UnmarshalText([]byte(*logLevel))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: programLevel})))

	ctx, cancelFunc := context.WithCancel(context.Background())

	etcdClient, err := clientv3.New(clientv3.Config{
		Context:   ctx,
		Endpoints: strings.Split(*etcdEndpoints, ","),
	})
	if err != nil {
		slog.Error("failed to connect to primary zone storage", "error", err)
		return
	}
	primary := zone.NewEtcdStorage(etcdClient, *etcdPrefix)
	fallback, err := zone.NewFileStorage(*localData)
	if err != nil {
		slog.Error("failed initialize fallback local storage", "error", err)
		return
	}

	nameserver.New(ctx, *dnsListen, primary, fallback)
	admin.New(ctx, *httpListen, primary, strings.Split(*apiKeys, ","))

	// Shutdown on SIGINT
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	slog.Info("Received SIGINT, shutting down...")
	cancelFunc()
}
