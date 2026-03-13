package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/romashqua-labs/outpost/internal/config"
	"github.com/romashqua-labs/outpost/internal/core"
	"github.com/romashqua-labs/outpost/internal/db"
	"github.com/romashqua-labs/outpost/internal/observability"
	"github.com/romashqua-labs/outpost/pkg/version"
)

func main() {
	cfg := config.Load()

	logger := observability.NewLogger(cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting outpost-core",
		"version", version.Version,
		"http_addr", cfg.Server.HTTPAddr,
		"grpc_addr", cfg.Server.GRPCAddr,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.New(ctx, cfg.Database)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	logger.Info("connected to database", "host", cfg.Database.Host, "name", cfg.Database.Name)

	srv := core.NewServer(cfg, pool, logger)
	if err := srv.Start(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("outpost-core stopped")
}
