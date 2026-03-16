package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/romashqua/outpost/internal/config"
	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

type Agent struct {
	cfg    *config.Config
	logger *slog.Logger

	mu     sync.Mutex
	conn   *grpc.ClientConn
	client gatewayv1.GatewayServiceClient

	wg *WGManager       // nil if WireGuard interface not available (e.g. dev mode)
	fw *FirewallManager // nil if iptables not available
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
				a.closeConn()
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
	defer a.closeConn()

	a.logger.Info("received gateway configuration",
		"gateway_id", cfg.GatewayId,
		"network", cfg.NetworkName,
		"peers", len(cfg.Peers),
		"s2s_tunnels", len(cfg.S2STunnels),
	)

	// Try to initialize WireGuard interface manager.
	ifaceName := "wg0"
	if a.cfg.Gateway.InterfaceName != "" {
		ifaceName = a.cfg.Gateway.InterfaceName
	}
	wgMgr, err := NewWGManager(ifaceName, a.logger)
	if err != nil {
		a.logger.Warn("WireGuard manager unavailable — peer updates will be logged only", "error", err)
	} else {
		a.wg = wgMgr
		defer wgMgr.Close()

		// Apply initial peers from config.
		for _, p := range cfg.Peers {
			if addErr := wgMgr.AddPeer(p.PublicKey, p.AllowedIps, p.Endpoint, int(p.PersistentKeepalive)); addErr != nil {
				a.logger.Warn("failed to add initial peer", "pubkey", p.PublicKey, "error", addErr)
			}
		}
	}

	// Initialize firewall manager for ACL enforcement.
	fwMgr := NewFirewallManager(a.logger)
	if err := fwMgr.Init(); err != nil {
		a.logger.Warn("firewall manager unavailable — ACL enforcement disabled", "error", err)
	} else {
		a.fw = fwMgr
		defer fwMgr.Cleanup()

		// Apply initial firewall config from GetConfig response.
		if cfg.Firewall != nil {
			if fwErr := fwMgr.Apply(cfg.Firewall); fwErr != nil {
				a.logger.Warn("failed to apply initial firewall config", "error", fwErr)
			}
		}
	}

	// Start sync stream.
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.syncLoop(ctx)
	}()

	// Start heartbeat loop — stops when ctx is cancelled.
	go a.heartbeatLoop(ctx)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		a.logger.Info("gateway agent shutting down")
		return nil
	}
}

// closeConn safely closes the current gRPC connection.
func (a *Agent) closeConn() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
		a.client = nil
	}
}

func (a *Agent) connect(ctx context.Context) error {
	// Close previous connection if any (prevents leak on reconnect).
	a.closeConn()

	transportCreds, err := a.buildTransportCredentials()
	if err != nil {
		return fmt.Errorf("build transport credentials: %w", err)
	}

	conn, err := grpc.NewClient(
		a.cfg.Gateway.CoreAddr,
		grpc.WithTransportCredentials(transportCreds),
	)
	if err != nil {
		return fmt.Errorf("dial core: %w", err)
	}

	a.mu.Lock()
	a.conn = conn
	a.client = gatewayv1.NewGatewayServiceClient(conn)
	a.mu.Unlock()

	a.logger.Info("connected to core", "addr", a.cfg.Gateway.CoreAddr, "tls", a.cfg.Gateway.TLSEnabled)
	return nil
}

// buildTransportCredentials constructs gRPC transport credentials based on
// the gateway TLS configuration. When TLS is disabled (default for dev),
// insecure credentials are used. When enabled, it supports:
//   - Server-only TLS (just TLSCAFile or system CA pool)
//   - Mutual TLS (mTLS) when TLSCertFile and TLSKeyFile are provided
//   - TLSInsecureSkipVerify for testing environments
func (a *Agent) buildTransportCredentials() (credentials.TransportCredentials, error) {
	if !a.cfg.Gateway.TLSEnabled {
		a.logger.Debug("TLS disabled, using insecure credentials")
		return insecure.NewCredentials(), nil
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Load client certificate and key for mTLS if both are provided.
	if a.cfg.Gateway.TLSCertFile != "" && a.cfg.Gateway.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(a.cfg.Gateway.TLSCertFile, a.cfg.Gateway.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
		a.logger.Info("mTLS enabled with client certificate",
			"cert", a.cfg.Gateway.TLSCertFile,
			"key", a.cfg.Gateway.TLSKeyFile,
		)
	}

	// Load custom CA certificate if provided, otherwise use system CA pool.
	if a.cfg.Gateway.TLSCAFile != "" {
		caPEM, err := os.ReadFile(a.cfg.Gateway.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse CA certificate from %s", a.cfg.Gateway.TLSCAFile)
		}
		tlsCfg.RootCAs = caPool
		a.logger.Info("using custom CA certificate", "ca", a.cfg.Gateway.TLSCAFile)
	}

	if a.cfg.Gateway.TLSInsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		a.logger.Warn("TLS certificate verification disabled — do not use in production")
	}

	return credentials.NewTLS(tlsCfg), nil
}

func (a *Agent) authContext(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+a.cfg.Gateway.Token)
}

func (a *Agent) getClient() gatewayv1.GatewayServiceClient {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

func (a *Agent) fetchConfig(ctx context.Context) (*gatewayv1.GatewayConfig, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := a.getClient()
	if client == nil {
		return nil, fmt.Errorf("no gRPC client available")
	}

	resp, err := client.GetConfig(a.authContext(fetchCtx), &gatewayv1.ConfigRequest{
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
	client := a.getClient()
	if client == nil {
		return fmt.Errorf("no gRPC client available")
	}

	stream, err := client.Sync(a.authContext(ctx))
	if err != nil {
		return fmt.Errorf("open sync stream: %w", err)
	}
	defer stream.CloseSend()

	// Periodically collect WireGuard peer stats and send them to core.
	go a.statsSendLoop(ctx, stream)

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

// statsSendLoop periodically collects WireGuard peer stats and sends them
// to core via the bidirectional sync stream.
func (a *Agent) statsSendLoop(ctx context.Context, stream grpc.BidiStreamingClient[gatewayv1.GatewayEvent, gatewayv1.CoreEvent]) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.wg == nil {
				continue
			}

			peers, err := a.wg.GetPeerStats()
			if err != nil {
				a.logger.Warn("failed to collect peer stats", "error", err)
				continue
			}

			protoStats := make([]*commonv1.PeerStats, 0, len(peers))
			for _, p := range peers {
				ps := &commonv1.PeerStats{
					PublicKey: p.PublicKey.String(),
					RxBytes:  p.ReceiveBytes,
					TxBytes:  p.TransmitBytes,
				}
				if p.Endpoint != nil {
					ps.Endpoint = p.Endpoint.String()
				}
				if !p.LastHandshakeTime.IsZero() {
					ps.LastHandshake = timestamppb.New(p.LastHandshakeTime)
				}
				protoStats = append(protoStats, ps)
			}

			if err := stream.Send(&gatewayv1.GatewayEvent{
				Event: &gatewayv1.GatewayEvent_Stats{
					Stats: &gatewayv1.PeerStatsReport{
						Peers:       protoStats,
						CollectedAt: timestamppb.Now(),
					},
				},
			}); err != nil {
				a.logger.Warn("failed to send peer stats", "error", err)
				return
			}

			a.logger.Debug("sent peer stats to core", "peers", len(protoStats))
		}
	}
}

func (a *Agent) handlePeerUpdate(update *gatewayv1.PeerUpdate) {
	peer := update.GetPeer()
	pubkey := peer.GetPublicKey()
	a.logger.Info("peer update",
		"action", update.Action.String(),
		"pubkey", pubkey,
	)

	if a.wg == nil {
		return
	}

	switch update.Action {
	case gatewayv1.PeerUpdate_ACTION_ADD, gatewayv1.PeerUpdate_ACTION_MODIFY:
		if err := a.wg.AddPeer(pubkey, peer.AllowedIps, peer.Endpoint, int(peer.PersistentKeepalive)); err != nil {
			a.logger.Error("failed to add/modify peer", "pubkey", pubkey, "error", err)
		}
	case gatewayv1.PeerUpdate_ACTION_REMOVE:
		if err := a.wg.RemovePeer(pubkey); err != nil {
			a.logger.Error("failed to remove peer", "pubkey", pubkey, "error", err)
		}
	}
}

func (a *Agent) handleS2SUpdate(update *gatewayv1.S2SUpdate) {
	tunnel := update.GetTunnel()
	if tunnel == nil {
		a.logger.Warn("s2s update with nil tunnel config")
		return
	}

	a.logger.Info("s2s update",
		"action", update.Action.String(),
		"tunnel_id", tunnel.GetTunnelId(),
		"interface", tunnel.GetInterfaceName(),
		"peers", len(tunnel.GetPeers()),
	)

	if a.wg == nil {
		a.logger.Warn("s2s update ignored: WireGuard manager not available")
		return
	}

	switch update.Action {
	case gatewayv1.S2SUpdate_ACTION_ADD_TUNNEL, gatewayv1.S2SUpdate_ACTION_UPDATE_ROUTES, gatewayv1.S2SUpdate_ACTION_UPDATE_PEERS:
		// Add/update all S2S peers on the main WG interface.
		// In production, these would go on a separate wg-s2s interface,
		// but for now we use the same wg0 interface for simplicity.
		for _, peer := range tunnel.GetPeers() {
			if peer.GetPublicKey() == "" {
				continue
			}
			if err := a.wg.AddPeer(
				peer.GetPublicKey(),
				peer.GetAllowedIps(),
				peer.GetEndpoint(),
				int(peer.GetPersistentKeepalive()),
			); err != nil {
				a.logger.Error("failed to add S2S peer",
					"tunnel_id", tunnel.GetTunnelId(),
					"peer_gateway", peer.GetGatewayId(),
					"error", err)
			} else {
				a.logger.Info("S2S peer added",
					"tunnel_id", tunnel.GetTunnelId(),
					"peer_gateway", peer.GetGatewayId(),
					"allowed_ips", peer.GetAllowedIps())
			}
		}
	case gatewayv1.S2SUpdate_ACTION_REMOVE_TUNNEL:
		// Remove all S2S peers for this tunnel.
		for _, peer := range tunnel.GetPeers() {
			if peer.GetPublicKey() == "" {
				continue
			}
			if err := a.wg.RemovePeer(peer.GetPublicKey()); err != nil {
				a.logger.Error("failed to remove S2S peer", "error", err)
			}
		}
	}
}

func (a *Agent) handleFirewallUpdate(update *gatewayv1.FirewallUpdate) {
	a.logger.Info("firewall update", "rules", len(update.Config.GetRules()))

	if a.fw == nil {
		a.logger.Warn("firewall update ignored: firewall manager not available")
		return
	}

	if err := a.fw.Apply(update.Config); err != nil {
		a.logger.Error("failed to apply firewall update", "error", err)
	}
}

func (a *Agent) handleFullResync(resync *gatewayv1.FullResync) {
	cfg := resync.GetConfig()
	a.logger.Info("full resync",
		"peers", len(cfg.GetPeers()),
		"s2s_tunnels", len(cfg.GetS2STunnels()),
	)

	if a.wg == nil {
		return
	}

	for _, p := range cfg.GetPeers() {
		if err := a.wg.AddPeer(p.PublicKey, p.AllowedIps, p.Endpoint, int(p.PersistentKeepalive)); err != nil {
			a.logger.Warn("resync: failed to add peer", "pubkey", p.PublicKey, "error", err)
		}
	}

	// Apply S2S tunnel peers.
	for _, tunnel := range cfg.GetS2STunnels() {
		for _, peer := range tunnel.GetPeers() {
			if peer.GetPublicKey() == "" {
				continue
			}
			if err := a.wg.AddPeer(
				peer.GetPublicKey(),
				peer.GetAllowedIps(),
				peer.GetEndpoint(),
				int(peer.GetPersistentKeepalive()),
			); err != nil {
				a.logger.Warn("resync: failed to add S2S peer",
					"tunnel_id", tunnel.GetTunnelId(),
					"peer_gateway", peer.GetGatewayId(),
					"error", err)
			}
		}
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			client := a.getClient()
			if client == nil {
				a.logger.Warn("heartbeat skipped: no client")
				continue
			}
			if _, err := client.Heartbeat(a.authContext(ctx), &gatewayv1.HeartbeatRequest{}); err != nil {
				a.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}
