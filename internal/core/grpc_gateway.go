package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

type gatewayService struct {
	gatewayv1.UnimplementedGatewayServiceServer
	pool   *pgxpool.Pool
	logger *slog.Logger
	hub    *StreamHub
}

func registerGatewayService(srv *grpc.Server, pool *pgxpool.Pool, logger *slog.Logger, hub *StreamHub) {
	gatewayv1.RegisterGatewayServiceServer(srv, &gatewayService{
		pool:   pool,
		logger: logger,
		hub:    hub,
	})
}

func (s *gatewayService) GetConfig(ctx context.Context, req *gatewayv1.ConfigRequest) (*gatewayv1.GatewayConfig, error) {
	token := req.GetGatewayToken()
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "gateway token is required")
	}
	tokenPreview := token
	if len(tokenPreview) > 8 {
		tokenPreview = tokenPreview[:8] + "..."
	}
	s.logger.Info("gateway requesting config", "token", tokenPreview)

	// Hash the token to compare against the stored token_hash.
	h := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(h[:])

	// Look up gateway by token hash.
	var gwID, gwName string
	var listenPort int32
	var addresses []string
	err := s.pool.QueryRow(ctx,
		`SELECT g.id, g.name, n.port, ARRAY[host(n.address) || '/' || masklen(n.address)]
		 FROM gateways g
		 JOIN networks n ON n.id = g.network_id
		 WHERE g.token_hash = $1 AND g.is_active = true`,
		tokenHash,
	).Scan(&gwID, &gwName, &listenPort, &addresses)
	if err != nil {
		s.logger.Warn("gateway config lookup failed", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid or inactive gateway token")
	}

	// Fetch all approved device peers for this gateway's network.
	peers, err := s.fetchPeers(ctx, gwID)
	if err != nil {
		s.logger.Error("failed to fetch peers for config", "error", err, "gateway_id", gwID)
	}

	// Fetch S2S tunnel configs for this gateway.
	s2sTunnels := s.fetchS2STunnels(ctx, gwID)

	return &gatewayv1.GatewayConfig{
		GatewayId:   gwID,
		NetworkName: gwName,
		ListenPort:  listenPort,
		Addresses:   addresses,
		Peers:       peers,
		S2STunnels:  s2sTunnels,
	}, nil
}

// fetchPeers returns approved device peers for the gateway's network.
func (s *gatewayService) fetchPeers(ctx context.Context, gatewayID string) ([]*commonv1.Peer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT d.wireguard_pubkey, host(d.assigned_ip) || '/32'
		 FROM devices d
		 JOIN gateways g ON g.network_id = d.network_id
		 WHERE g.id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''`,
		gatewayID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*commonv1.Peer
	for rows.Next() {
		var pubkey, allowedIP string
		if err := rows.Scan(&pubkey, &allowedIP); err != nil {
			return nil, err
		}
		peers = append(peers, &commonv1.Peer{
			PublicKey:            pubkey,
			AllowedIps:          []string{allowedIP},
			PersistentKeepalive: 25,
		})
	}
	return peers, rows.Err()
}

// fetchS2STunnels returns S2S tunnel configs for a gateway.
func (s *gatewayService) fetchS2STunnels(ctx context.Context, gatewayID string) []*gatewayv1.S2STunnelConfig {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.name, t.topology
		 FROM s2s_tunnels t
		 JOIN s2s_tunnel_members m ON m.tunnel_id = t.id
		 WHERE m.gateway_id::text = $1 AND t.is_active = true`, gatewayID)
	if err != nil {
		s.logger.Warn("failed to fetch S2S tunnels", "gateway_id", gatewayID, "error", err)
		return nil
	}
	defer rows.Close()

	var configs []*gatewayv1.S2STunnelConfig
	for rows.Next() {
		var tunnelID, tunnelName, topology string
		if err := rows.Scan(&tunnelID, &tunnelName, &topology); err != nil {
			continue
		}

		peerRows, err := s.pool.Query(ctx,
			`SELECT m.gateway_id, g.wireguard_pubkey, g.endpoint,
			        ARRAY(SELECT unnest(m.local_subnets)::text)
			 FROM s2s_tunnel_members m
			 JOIN gateways g ON g.id = m.gateway_id
			 WHERE m.tunnel_id = $1 AND m.gateway_id::text != $2`,
			tunnelID, gatewayID)
		if err != nil {
			continue
		}

		var peers []*gatewayv1.S2SPeer
		for peerRows.Next() {
			var gwID, pubkey, endpoint string
			var subnets []string
			if err := peerRows.Scan(&gwID, &pubkey, &endpoint, &subnets); err != nil {
				continue
			}
			peers = append(peers, &gatewayv1.S2SPeer{
				GatewayId:           gwID,
				PublicKey:           pubkey,
				Endpoint:            endpoint,
				AllowedIps:          subnets,
				PersistentKeepalive: 25,
			})
		}
		peerRows.Close()

		configs = append(configs, &gatewayv1.S2STunnelConfig{
			TunnelId:      tunnelID,
			InterfaceName: "wg-s2s-" + tunnelName,
			Peers:         peers,
		})
	}

	return configs
}

// authenticateStream extracts the gateway token from gRPC metadata,
// hashes it, and returns the gateway ID if valid.
func (s *gatewayService) authenticateStream(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return "", fmt.Errorf("no authorization header")
	}
	token := strings.TrimPrefix(vals[0], "Bearer ")
	if token == "" {
		return "", fmt.Errorf("empty token")
	}

	h := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(h[:])

	var gwID string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM gateways WHERE token_hash = $1 AND is_active = true`,
		tokenHash,
	).Scan(&gwID)
	if err != nil {
		return "", fmt.Errorf("token lookup: %w", err)
	}
	return gwID, nil
}

func (s *gatewayService) Sync(stream grpc.BidiStreamingServer[gatewayv1.GatewayEvent, gatewayv1.CoreEvent]) error {
	s.logger.Info("gateway sync stream opened")

	// Authenticate gateway via token in gRPC metadata.
	gwID, err := s.authenticateStream(stream.Context())
	if err != nil {
		s.logger.Warn("gateway sync auth failed", "error", err)
		return status.Error(codes.Unauthenticated, "invalid gateway token")
	}

	cleanup := s.hub.Register(gwID, stream)
	defer cleanup()

	// Send a full resync with current config so gateway is up to date.
	peers, err := s.fetchPeers(stream.Context(), gwID)
	s2sTunnels := s.fetchS2STunnels(stream.Context(), gwID)
	if err == nil && (len(peers) > 0 || len(s2sTunnels) > 0) {
		_ = stream.Send(&gatewayv1.CoreEvent{
			Event: &gatewayv1.CoreEvent_FullResync{
				FullResync: &gatewayv1.FullResync{
					Config: &gatewayv1.GatewayConfig{
						Peers:      peers,
						S2STunnels: s2sTunnels,
					},
				},
			},
		})
	}

	// Keep the stream open — receive events from gateway.
	for {
		event, err := stream.Recv()
		if err != nil {
			s.logger.Info("gateway sync stream closed", "error", err)
			return err
		}

		switch {
		case event.GetStats() != nil:
			s.handlePeerStats(stream.Context(), gwID, event.GetStats())
		case event.GetS2SHealth() != nil:
			s.logger.Debug("received s2s health report")
		case event.GetStatus() != nil:
			s.logger.Debug("received gateway status",
				"active_peers", event.GetStatus().GetActivePeers())
		}
	}
}

// handlePeerStats updates last_handshake for devices based on gateway stats,
// records peer_stats and flow_records for bandwidth analytics.
func (s *gatewayService) handlePeerStats(ctx context.Context, gatewayID string, stats *gatewayv1.PeerStatsReport) {
	for _, ps := range stats.GetPeers() {
		pubkey := ps.GetPublicKey()

		// Update last_handshake if present.
		if ps.GetLastHandshake() != nil {
			ht := ps.GetLastHandshake().AsTime()
			if !ht.IsZero() {
				_, _ = s.pool.Exec(ctx,
					`UPDATE devices SET last_handshake = $1 WHERE wireguard_pubkey = $2`,
					ht, pubkey,
				)
			}
		}

		// Record peer stats (for bandwidth chart).
		var endpoint string
		if ps.GetEndpoint() != "" {
			endpoint = ps.GetEndpoint()
		}
		var lastHS *time.Time
		if ps.GetLastHandshake() != nil {
			t := ps.GetLastHandshake().AsTime()
			if !t.IsZero() {
				lastHS = &t
			}
		}
		_, err := s.pool.Exec(ctx,
			`INSERT INTO peer_stats (gateway_id, device_id, rx_bytes, tx_bytes, last_handshake, endpoint)
			 SELECT $1::uuid, d.id, $3, $4, $5, $6
			 FROM devices d WHERE d.wireguard_pubkey = $2`,
			gatewayID, pubkey, ps.GetRxBytes(), ps.GetTxBytes(), lastHS, endpoint,
		)
		if err != nil {
			s.logger.Warn("failed to insert peer_stats", "pubkey", pubkey, "error", err)
		}

		// Record flow record (for analytics/bandwidth chart on dashboard).
		_, err = s.pool.Exec(ctx,
			`INSERT INTO flow_records (gateway_id, device_id, user_id, src_ip, dst_ip, protocol, dst_port, bytes_sent, bytes_recv)
			 SELECT $1::uuid, d.id, d.user_id, COALESCE(d.assigned_ip, '0.0.0.0'::inet), '0.0.0.0'::inet, 'wg', 0, $3, $4
			 FROM devices d WHERE d.wireguard_pubkey = $2`,
			gatewayID, pubkey, ps.GetTxBytes(), ps.GetRxBytes(),
		)
		if err != nil {
			s.logger.Warn("failed to insert flow_record", "pubkey", pubkey, "error", err)
		}
	}
}

func (s *gatewayService) Heartbeat(ctx context.Context, req *gatewayv1.HeartbeatRequest) (*emptypb.Empty, error) {
	// Authenticate gateway via token in gRPC metadata.
	authenticatedGwID, err := s.authenticateStream(ctx)
	if err != nil {
		s.logger.Warn("gateway heartbeat auth failed", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid gateway token")
	}

	gwID := req.GetGatewayId()

	// Verify the requested gateway_id matches the authenticated gateway.
	if gwID != "" && gwID != authenticatedGwID {
		s.logger.Warn("gateway heartbeat id mismatch",
			"requested", gwID, "authenticated", authenticatedGwID)
		return nil, status.Error(codes.PermissionDenied, "gateway_id does not match authenticated gateway")
	}

	// Use the authenticated gateway ID for the update.
	s.logger.Debug("gateway heartbeat", "gateway_id", authenticatedGwID)

	if _, err := s.pool.Exec(ctx,
		`UPDATE gateways SET last_seen = now() WHERE id::text = $1`, authenticatedGwID); err != nil {
		s.logger.Error("failed to update gateway last_seen", "gateway_id", authenticatedGwID, "error", err)
	}

	return &emptypb.Empty{}, nil
}
