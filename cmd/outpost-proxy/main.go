package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/romashqua/outpost/internal/config"
	"github.com/romashqua/outpost/internal/observability"
	"github.com/romashqua/outpost/internal/proxy"
	"github.com/romashqua/outpost/pkg/version"
)

func main() {
	cfg := config.Load()

	logger := observability.NewLogger(cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting outpost-proxy",
		"version", version.Version,
		"addr", cfg.Proxy.ListenAddr,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := proxy.NewServer(cfg, logger)
	if err := srv.Run(ctx); err != nil {
		logger.Error("proxy server error", "error", err)
		os.Exit(1)
	}

	logger.Info("outpost-proxy stopped")
}
