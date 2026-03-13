package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/romashqua-labs/outpost/internal/config"
	"github.com/romashqua-labs/outpost/internal/observability"
	"github.com/romashqua-labs/outpost/pkg/version"
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

	// Proxy implementation will be added in Phase 5.
	_ = ctx
	_ = cfg

	logger.Info("outpost-proxy stopped")
}
