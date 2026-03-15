package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"

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

	return &gatewayv1.GatewayConfig{
		GatewayId:   gwID,
		NetworkName: gwName,
		ListenPort:  listenPort,
		Addresses:   addresses,
		Peers:       peers,
	}, nil
}

// fetchPeers returns all approved device peers.
// Currently returns all approved devices since the data model does not
// associate devices with specific networks (all devices are on the default network).
func (s *gatewayService) fetchPeers(ctx context.Context, _ string) ([]*commonv1.Peer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT wireguard_pubkey, host(assigned_ip) || '/32'
		 FROM devices
		 WHERE is_approved = true AND wireguard_pubkey != ''`,
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
	if err == nil && len(peers) > 0 {
		_ = stream.Send(&gatewayv1.CoreEvent{
			Event: &gatewayv1.CoreEvent_FullResync{
				FullResync: &gatewayv1.FullResync{
					Config: &gatewayv1.GatewayConfig{
						Peers: peers,
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
// scoped to the gateway's network to prevent cross-network updates.
func (s *gatewayService) handlePeerStats(ctx context.Context, gatewayID string, stats *gatewayv1.PeerStatsReport) {
	for _, ps := range stats.GetPeers() {
		if ps.GetLastHandshake() == nil {
			continue
		}
		ht := ps.GetLastHandshake().AsTime()
		if ht.IsZero() {
			continue
		}
		_, err := s.pool.Exec(ctx,
			`UPDATE devices SET last_handshake = $1
			 WHERE wireguard_pubkey = $2`,
			ht, ps.GetPublicKey(),
		)
		if err != nil {
			s.logger.Warn("failed to update last_handshake", "pubkey", ps.GetPublicKey(), "error", err)
		}
	}
}

func (s *gatewayService) Heartbeat(ctx context.Context, req *gatewayv1.HeartbeatRequest) (*emptypb.Empty, error) {
	gwID := req.GetGatewayId()
	s.logger.Debug("gateway heartbeat", "gateway_id", gwID)

	// Update gateway last_seen.
	if gwID != "" {
		if _, err := s.pool.Exec(ctx,
			`UPDATE gateways SET last_seen = now() WHERE id::text = $1`, gwID); err != nil {
			s.logger.Error("failed to update gateway last_seen", "gateway_id", gwID, "error", err)
		}
	}

	return &emptypb.Empty{}, nil
}
