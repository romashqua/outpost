package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/romashqua/outpost/internal/config"
	"github.com/romashqua/outpost/internal/gateway"
	"github.com/romashqua/outpost/internal/observability"
	"github.com/romashqua/outpost/pkg/version"
)

func main() {
	cfg := config.Load()

	logger := observability.NewLogger(cfg.Log.Level, cfg.Log.Format)
	logger.Info("starting outpost-gateway",
		"version", version.Version,
		"core_addr", cfg.Gateway.CoreAddr,
		"tls_enabled", cfg.Gateway.TLSEnabled,
		"tls_mtls", cfg.Gateway.TLSCertFile != "" && cfg.Gateway.TLSKeyFile != "",
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	agent, err := gateway.NewAgent(cfg, logger)
	if err != nil {
		logger.Error("failed to create gateway agent", "error", err)
		os.Exit(1)
	}

	if err := agent.Run(ctx); err != nil {
		logger.Error("gateway agent error", "error", err)
		os.Exit(1)
	}

	logger.Info("outpost-gateway stopped")
}
