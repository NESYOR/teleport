/*
 * Teleport
 * Copyright (C) 2026 Gravitational, Inc.
 */

// Command teleport-livemap connects to a Teleport Auth or Proxy server,
// subscribes to real-time Auth resource watch events (KindAuthServer, KindProxy,
// KindNode, KindDatabaseServer, KindKubeServer, KindAppServer,
// KindTunnelConnection, KindSessionTracker), and serves a live cluster topology
// and data-flow map over HTTP.
//
// Usage against a running Teleport cluster:
//
//	tctl auth sign --user=admin --out=identity.pem
//	go run ./lib/observability/livemap/cmd/teleport-livemap \
//	  --addr=localhost:3025 \
//	  --identity=identity.pem \
//	  --listen=127.0.0.1:8899
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/lib/observability/livemap"
)

func main() {
	var (
		addrsFlag    = flag.String("addr", "127.0.0.1:3025", "Comma-separated Teleport Auth or Proxy server addresses")
		identityPath = flag.String("identity", "", "Path to Teleport identity file (generated via 'tctl auth sign -o identity.pem')")
		listenAddr   = flag.String("listen", "127.0.0.1:8899", "HTTP listen address for the Live Topology Map UI")
	)
	flag.Parse()

	if *identityPath == "" {
		slog.Error("missing --identity flag (generate one with: tctl auth sign -o identity.pem)")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addrs := strings.Split(*addrsFlag, ",")
	clt, err := client.New(ctx, client.Config{
		Addrs: addrs,
		Credentials: []client.Credentials{
			client.LoadIdentityFile(*identityPath),
		},
	})
	if err != nil {
		slog.Error("failed to connect to Teleport Auth/Proxy", "addrs", addrs, "error", err)
		os.Exit(1)
	}
	defer clt.Close()

	hub := livemap.NewHub()
	consumer := livemap.NewConsumer(hub)

	go func() {
		slog.Info("subscribed to Teleport Auth event stream (NewWatcher)", "addrs", addrs)
		if err := consumer.WatchCluster(ctx, clt); err != nil {
			slog.Error("Auth event watcher stopped", "error", err)
		}
	}()

	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: livemap.NewHTTPHandler(hub, nil),
	}

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	slog.Info("Teleport Live Topology Map listening", "url", "http://"+*listenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
