package main

import (
	"context"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	outpost "github.com/romashqua/outpost"
	"github.com/romashqua/outpost/internal/config"
	"github.com/romashqua/outpost/internal/core"
	"github.com/romashqua/outpost/internal/db"
	"github.com/romashqua/outpost/internal/observability"
	"github.com/romashqua/outpost/pkg/version"
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

	// Run database migrations.
	migrationsFS, err := fs.Sub(outpost.Migrations, "migrations")
	if err != nil {
		logger.Error("failed to load migrations", "error", err)
		os.Exit(1)
	}
	if err := db.RunMigrations(pool, migrationsFS, logger); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	srv := core.NewServer(cfg, pool, logger)
	if err := srv.Start(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("outpost-core stopped")
}
