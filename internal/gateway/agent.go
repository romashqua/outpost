package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/romashqua/outpost/internal/config"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

type Agent struct {
	cfg    *config.Config
	logger *slog.Logger
	conn   *grpc.ClientConn
	client gatewayv1.GatewayServiceClient
}

func NewAgent(cfg *config.Config, logger *slog.Logger) (*Agent, error) {
	return &Agent{
		cfg:    cfg,
		logger: logger,
	}, nil
}

func (a *Agent) Run(ctx context.Context) error {
	// Retry connection to core with backoff.
	var cfg *gatewayv1.GatewayConfig
	for attempt := 1; ; attempt++ {
		if err := a.connect(ctx); err != nil {
			a.logger.Warn("failed to connect to core, retrying",
				"error", err, "attempt", attempt)
		} else {
			var err error
			cfg, err = a.fetchConfig(ctx)
			if err != nil {
				a.logger.Warn("failed to fetch config, retrying",
					"error", err, "attempt", attempt)
				a.conn.Close()
			} else {
				break
			}
		}

		delay := time.Duration(min(attempt*2, 30)) * time.Second
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
	defer a.conn.Close()

	a.logger.Info("received gateway configuration",
		"gateway_id", cfg.GatewayId,
		"network", cfg.NetworkName,
		"peers", len(cfg.Peers),
		"s2s_tunnels", len(cfg.S2STunnels),
	)

	// Start sync stream.
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.syncLoop(ctx)
	}()

	// Start heartbeat loop.
	go a.heartbeatLoop(ctx)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		a.logger.Info("gateway agent shutting down")
		return nil
	}
}

func (a *Agent) connect(ctx context.Context) error {
	// TODO: Add TLS credentials for production.
	conn, err := grpc.NewClient(
		a.cfg.Gateway.CoreAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial core: %w", err)
	}

	a.conn = conn
	a.client = gatewayv1.NewGatewayServiceClient(conn)
	a.logger.Info("connected to core", "addr", a.cfg.Gateway.CoreAddr)
	return nil
}

func (a *Agent) authContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+a.cfg.Gateway.Token)
}

func (a *Agent) fetchConfig(ctx context.Context) (*gatewayv1.GatewayConfig, error) {
	resp, err := a.client.GetConfig(a.authContext(ctx), &gatewayv1.ConfigRequest{
		GatewayToken: a.cfg.Gateway.Token,
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (a *Agent) syncLoop(ctx context.Context) error {
	for {
		if err := a.runSync(ctx); err != nil {
			a.logger.Error("sync stream error, reconnecting", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(5 * time.Second):
			// Reconnect after delay.
		}
	}
}

func (a *Agent) runSync(ctx context.Context) error {
	stream, err := a.client.Sync(a.authContext(ctx))
	if err != nil {
		return fmt.Errorf("open sync stream: %w", err)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}

		switch e := event.Event.(type) {
		case *gatewayv1.CoreEvent_PeerUpdate:
			a.handlePeerUpdate(e.PeerUpdate)
		case *gatewayv1.CoreEvent_S2SUpdate:
			a.handleS2SUpdate(e.S2SUpdate)
		case *gatewayv1.CoreEvent_FirewallUpdate:
			a.handleFirewallUpdate(e.FirewallUpdate)
		case *gatewayv1.CoreEvent_FullResync:
			a.handleFullResync(e.FullResync)
		}
	}
}

func (a *Agent) handlePeerUpdate(update *gatewayv1.PeerUpdate) {
	a.logger.Info("peer update",
		"action", update.Action.String(),
		"pubkey", update.Peer.GetPublicKey(),
	)
	// WireGuard interface management will be implemented with wireguard package.
}

func (a *Agent) handleS2SUpdate(update *gatewayv1.S2SUpdate) {
	a.logger.Info("s2s update",
		"action", update.Action.String(),
		"tunnel", update.Tunnel.GetTunnelId(),
	)
}

func (a *Agent) handleFirewallUpdate(update *gatewayv1.FirewallUpdate) {
	a.logger.Info("firewall update", "rules", len(update.Config.GetRules()))
}

func (a *Agent) handleFullResync(resync *gatewayv1.FullResync) {
	a.logger.Info("full resync",
		"peers", len(resync.Config.GetPeers()),
		"s2s_tunnels", len(resync.Config.GetS2STunnels()),
	)
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := a.client.Heartbeat(a.authContext(ctx), &gatewayv1.HeartbeatRequest{}); err != nil {
				a.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}
