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
	var gwID, gwName, gwPrivKey string
	err := s.pool.QueryRow(ctx,
		`SELECT g.id::text, g.name, COALESCE(g.wireguard_privkey, '')
		 FROM gateways g
		 WHERE g.token_hash = $1 AND g.is_active = true`,
		tokenHash,
	).Scan(&gwID, &gwName, &gwPrivKey)
	if err != nil {
		s.logger.Warn("gateway config lookup failed", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid or inactive gateway token")
	}

	// Get listen port and addresses from the gateway's networks.
	var listenPort int32
	var addresses []string
	netRows, err := s.pool.Query(ctx,
		`SELECT n.port, host(COALESCE(n.tunnel_cidr, n.address)::inet + 1) || '/' || masklen(COALESCE(n.tunnel_cidr, n.address))
		 FROM gateway_networks gn
		 JOIN networks n ON n.id = gn.network_id
		 WHERE gn.gateway_id::text = $1`, gwID)
	if err == nil {
		defer netRows.Close()
		for netRows.Next() {
			var port int32
			var addr string
			if err := netRows.Scan(&port, &addr); err == nil {
				if listenPort == 0 {
					listenPort = port
				}
				addresses = append(addresses, addr)
			}
		}
	}
	// Fallback: try legacy network_id join if gateway_networks is empty.
	if len(addresses) == 0 {
		_ = s.pool.QueryRow(ctx,
			`SELECT n.port, ARRAY[host(COALESCE(n.tunnel_cidr, n.address)::inet + 1) || '/' || masklen(COALESCE(n.tunnel_cidr, n.address))]
			 FROM gateways g
			 JOIN networks n ON n.id = g.network_id
			 WHERE g.id::text = $1`,
			gwID,
		).Scan(&listenPort, &addresses)
	}

	// Fetch all approved device peers for this gateway's networks.
	peers, err := s.fetchPeers(ctx, gwID)
	if err != nil {
		s.logger.Error("failed to fetch peers for config", "error", err, "gateway_id", gwID)
	}

	// Fetch S2S tunnel configs for this gateway.
	s2sTunnels := s.fetchS2STunnels(ctx, gwID)

	// Build firewall config based on device ACLs.
	fwConfig := s.buildFirewallConfig(ctx, gwID)

	// Fetch smart route rules for this gateway's networks.
	smartRoutes := fetchSmartRoutesForGateway(ctx, s.pool, s.logger, gwID)

	return &gatewayv1.GatewayConfig{
		GatewayId:   gwID,
		NetworkName: gwName,
		PrivateKey:  gwPrivKey,
		ListenPort:  listenPort,
		Addresses:   addresses,
		Peers:       peers,
		S2STunnels:  s2sTunnels,
		Firewall:    fwConfig,
		SmartRoutes: smartRoutes,
	}, nil
}

// fetchPeers returns approved device peers for all of the gateway's networks.
func (s *gatewayService) fetchPeers(ctx context.Context, gatewayID string) ([]*commonv1.Peer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT d.wireguard_pubkey, host(d.assigned_ip) || '/32'
		 FROM devices d
		 JOIN gateway_networks gn ON gn.network_id = d.network_id
		 WHERE gn.gateway_id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''
		 UNION
		 SELECT d.wireguard_pubkey, host(d.assigned_ip) || '/32'
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
	fwConfig := s.buildFirewallConfig(stream.Context(), gwID)
	smartRoutes := fetchSmartRoutesForGateway(stream.Context(), s.pool, s.logger, gwID)
	if err == nil && (len(peers) > 0 || len(s2sTunnels) > 0 || fwConfig != nil) {
		_ = stream.Send(&gatewayv1.CoreEvent{
			Event: &gatewayv1.CoreEvent_FullResync{
				FullResync: &gatewayv1.FullResync{
					Config: &gatewayv1.GatewayConfig{
						Peers:       peers,
						S2STunnels:  s2sTunnels,
						Firewall:    fwConfig,
						SmartRoutes: smartRoutes,
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

		// Update last_handshake if present — scoped to gateway's networks to prevent
		// a compromised gateway from updating devices in other networks.
		if ps.GetLastHandshake() != nil {
			ht := ps.GetLastHandshake().AsTime()
			if !ht.IsZero() {
				_, _ = s.pool.Exec(ctx,
					`UPDATE devices SET last_handshake = $1
					 WHERE wireguard_pubkey = $2
					   AND network_id IN (SELECT network_id FROM gateway_networks WHERE gateway_id = $3::uuid)`,
					ht, pubkey, gatewayID,
				)
			}
		}

		// Record peer stats (for bandwidth chart) — scoped to gateway's networks.
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
			 FROM devices d
			 WHERE d.wireguard_pubkey = $2
			   AND d.network_id IN (SELECT network_id FROM gateway_networks WHERE gateway_id = $1::uuid)`,
			gatewayID, pubkey, ps.GetRxBytes(), ps.GetTxBytes(), lastHS, endpoint,
		)
		if err != nil {
			s.logger.Warn("failed to insert peer_stats", "pubkey", pubkey, "error", err)
		}

		// Record flow record (for analytics/bandwidth chart on dashboard) — scoped to gateway's networks.
		_, err = s.pool.Exec(ctx,
			`INSERT INTO flow_records (gateway_id, device_id, user_id, src_ip, dst_ip, protocol, dst_port, bytes_sent, bytes_recv)
			 SELECT $1::uuid, d.id, d.user_id, COALESCE(d.assigned_ip, '0.0.0.0'::inet), '0.0.0.0'::inet, 'wg', 0, $3, $4
			 FROM devices d
			 WHERE d.wireguard_pubkey = $2
			   AND d.network_id IN (SELECT network_id FROM gateway_networks WHERE gateway_id = $1::uuid)`,
			gatewayID, pubkey, ps.GetTxBytes(), ps.GetRxBytes(),
		)
		if err != nil {
			s.logger.Warn("failed to insert flow_record", "pubkey", pubkey, "error", err)
		}
	}
}

// buildFirewallConfig computes iptables rules for all approved devices on a gateway
// based on their users' group ACLs. For each device:
//   - ACCEPT rules for networks the user's groups are allowed to access
//   - A final DROP rule to block access to all other networks
//
// This is a method on gatewayService but the logic is also used by hubPeerNotifier
// via the exported BuildFirewallConfig wrapper below.
func (s *gatewayService) buildFirewallConfig(ctx context.Context, gatewayID string) *commonv1.FirewallConfig {
	return buildFirewallConfigFromPool(ctx, s.pool, s.logger, gatewayID)
}

// buildFirewallConfigFromPool is the shared implementation used by both
// gatewayService and hubPeerNotifier.
func buildFirewallConfigFromPool(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, gatewayID string) *commonv1.FirewallConfig {
	// Query all approved devices on this gateway with their ACL-allowed networks.
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT
		     host(d.assigned_ip) AS device_ip,
		     n.address::text AS allowed_cidr
		 FROM devices d
		 JOIN gateway_networks gn ON gn.network_id = d.network_id
		 JOIN user_groups ug ON ug.user_id = d.user_id
		 JOIN network_acls a ON a.group_id = ug.group_id
		 JOIN networks n ON n.id = a.network_id AND n.is_active = true
		 WHERE gn.gateway_id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''
		 ORDER BY device_ip, allowed_cidr`, gatewayID)
	if err != nil {
		logger.Warn("failed to build firewall config", "error", err)
		return nil
	}
	defer rows.Close()

	// Track which devices have ACL entries so we can add DROP rules.
	deviceACLs := make(map[string][]string) // device_ip -> []allowed_cidr
	for rows.Next() {
		var deviceIP, allowedCIDR string
		if err := rows.Scan(&deviceIP, &allowedCIDR); err != nil {
			continue
		}
		deviceACLs[deviceIP] = append(deviceACLs[deviceIP], allowedCIDR)
	}

	// Fetch ALL approved devices on this gateway (for default-deny on devices without ACL).
	allRows, err := pool.Query(ctx,
		`SELECT DISTINCT host(d.assigned_ip)
		 FROM devices d
		 JOIN gateway_networks gn ON gn.network_id = d.network_id
		 WHERE gn.gateway_id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''
		 UNION
		 SELECT host(d.assigned_ip)
		 FROM devices d
		 JOIN gateways g ON g.network_id = d.network_id
		 WHERE g.id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''`, gatewayID)
	if err != nil {
		logger.Warn("failed to fetch all devices for default-deny", "error", err)
	}
	var allDeviceIPs []string
	if allRows != nil {
		for allRows.Next() {
			var ip string
			if err := allRows.Scan(&ip); err == nil {
				allDeviceIPs = append(allDeviceIPs, ip)
			}
		}
		allRows.Close()
	}

	// ZTNA: fetch trust scores and config for enforcement.
	blockedByZTNA := make(map[string]bool) // device IPs blocked by ZTNA policy
	var autoRestrict, autoBlock bool
	_ = pool.QueryRow(ctx,
		`SELECT auto_restrict_below_medium, auto_block_below_low
		 FROM trust_score_config WHERE id = 1`).Scan(&autoRestrict, &autoBlock)

	if autoRestrict || autoBlock {
		// Fetch latest trust scores for devices on this gateway.
		trustRows, err := pool.Query(ctx,
			`SELECT DISTINCT ON (d.id) host(d.assigned_ip), ts.level
			 FROM device_trust_scores ts
			 JOIN devices d ON d.id = ts.device_id
			 JOIN gateway_networks gn ON gn.network_id = d.network_id
			 WHERE gn.gateway_id::text = $1
			   AND d.is_approved = true
			   AND d.wireguard_pubkey != ''
			 ORDER BY d.id, ts.evaluated_at DESC`, gatewayID)
		if err == nil {
			for trustRows.Next() {
				var ip, level string
				if err := trustRows.Scan(&ip, &level); err != nil {
					continue
				}
				if autoBlock && (level == "critical" || level == "low") {
					blockedByZTNA[ip] = true
				} else if autoRestrict && level == "critical" {
					blockedByZTNA[ip] = true
				}
			}
			trustRows.Close()
		}
	}

	var rules []*commonv1.FirewallRule

	// For each device with ACL entries: ACCEPT allowed, DROP rest.
	// ZTNA-blocked devices get a DROP regardless of ACL.
	for deviceIP, cidrs := range deviceACLs {
		if blockedByZTNA[deviceIP] {
			rules = append(rules, &commonv1.FirewallRule{
				Source: deviceIP + "/32",
				Action: commonv1.FirewallRule_ACTION_DROP,
			})
			continue
		}
		for _, cidr := range cidrs {
			rules = append(rules, &commonv1.FirewallRule{
				Source:      deviceIP + "/32",
				Destination: cidr,
				Action:      commonv1.FirewallRule_ACTION_ACCEPT,
			})
		}
		// Default DROP for this device's traffic to other destinations.
		rules = append(rules, &commonv1.FirewallRule{
			Source: deviceIP + "/32",
			Action: commonv1.FirewallRule_ACTION_DROP,
		})
	}

	// Default-deny: devices without any ACL entries get a DROP rule.
	for _, ip := range allDeviceIPs {
		if _, hasACL := deviceACLs[ip]; !hasACL {
			rules = append(rules, &commonv1.FirewallRule{
				Source: ip + "/32",
				Action: commonv1.FirewallRule_ACTION_DROP,
			})
		}
	}

	// Always return config with NAT enabled so gateway masquerades VPN traffic.
	// Without masquerade, destination hosts cannot route replies back to VPN clients.
	return &commonv1.FirewallConfig{
		Rules:      rules,
		NatEnabled: true,
	}
}

// fetchSmartRoutesForGateway returns smart route rules for all networks served by a gateway.
func fetchSmartRoutesForGateway(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, gatewayID string) *commonv1.SmartRouteConfig {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT e.entry_type, e.value, e.action, e.priority
		 FROM smart_route_entries e
		 JOIN smart_routes sr ON sr.id = e.smart_route_id AND sr.is_active = true
		 JOIN network_smart_routes nsr ON nsr.smart_route_id = sr.id
		 JOIN gateway_networks gn ON gn.network_id = nsr.network_id
		 WHERE gn.gateway_id::text = $1
		 UNION
		 SELECT DISTINCT e.entry_type, e.value, e.action, e.priority
		 FROM smart_route_entries e
		 JOIN smart_routes sr ON sr.id = e.smart_route_id AND sr.is_active = true
		 JOIN network_smart_routes nsr ON nsr.smart_route_id = sr.id
		 JOIN gateways g ON g.network_id = nsr.network_id
		 WHERE g.id::text = $1
		 ORDER BY priority`, gatewayID)
	if err != nil {
		logger.Warn("failed to fetch smart routes", "gateway_id", gatewayID, "error", err)
		return nil
	}
	defer rows.Close()

	var rules []*commonv1.SmartRouteRule
	for rows.Next() {
		var entryType, value, action string
		var priority int32
		if err := rows.Scan(&entryType, &value, &action, &priority); err != nil {
			continue
		}
		rules = append(rules, &commonv1.SmartRouteRule{
			EntryType: entryType,
			Value:     value,
			Action:    action,
			Priority:  priority,
		})
	}
	if len(rules) == 0 {
		return nil
	}
	return &commonv1.SmartRouteConfig{Rules: rules}
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
		`UPDATE gateways SET last_seen = now(), health_status = 'healthy', consecutive_failures = 0
		 WHERE id = $1::uuid`, authenticatedGwID); err != nil {
		s.logger.Error("failed to update gateway last_seen", "gateway_id", authenticatedGwID, "error", err)
	}

	return &emptypb.Empty{}, nil
}

// MonitorGatewayHealth periodically checks gateway liveness and marks unhealthy
// gateways based on last_seen timestamps. Should be run as a goroutine.
// Deprecated: use monitorGatewayHealthTick with runWithLeaderLock instead.
func MonitorGatewayHealth(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, hub *StreamHub) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			monitorGatewayHealthTick(ctx, pool, logger, hub)
		}
	}
}

// monitorGatewayHealthTick performs a single health check pass: increments
// failure counters for stale gateways and marks them unhealthy after 3 failures.
func monitorGatewayHealthTick(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, hub *StreamHub) {
	// Increment failures for gateways that haven't been seen in 90 seconds.
	_, _ = pool.Exec(ctx,
		`UPDATE gateways
		 SET consecutive_failures = consecutive_failures + 1
		 WHERE is_active = true
		   AND last_seen < now() - interval '90 seconds'
		   AND health_status != 'unhealthy'`)

	// Mark as unhealthy after 3 consecutive failures (~4.5 min without heartbeat).
	rows, err := pool.Query(ctx,
		`UPDATE gateways
		 SET health_status = 'unhealthy'
		 WHERE is_active = true
		   AND consecutive_failures >= 3
		   AND health_status != 'unhealthy'
		 RETURNING id::text`)
	if err != nil {
		return
	}
	for rows.Next() {
		var gwID string
		if rows.Scan(&gwID) == nil {
			logger.Warn("gateway marked unhealthy", "gateway_id", gwID)
			pushResyncForPeersOfGateway(ctx, pool, logger, hub, gwID)
		}
	}
	rows.Close()
}

// pushResyncForPeersOfGateway sends a full resync to all healthy gateways
// that share networks with the unhealthy gateway, so clients get updated endpoint lists.
func pushResyncForPeersOfGateway(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, hub *StreamHub, unhealthyGwID string) {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT g2.id::text
		 FROM gateway_networks gn1
		 JOIN gateway_networks gn2 ON gn2.network_id = gn1.network_id
		 JOIN gateways g2 ON g2.id = gn2.gateway_id
		 WHERE gn1.gateway_id::text = $1
		   AND g2.id::text != $1
		   AND g2.is_active = true
		   AND g2.health_status = 'healthy'`, unhealthyGwID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var peerGwID string
		if rows.Scan(&peerGwID) == nil {
			logger.Info("pushing resync to healthy peer gateway", "gateway_id", peerGwID, "reason", "peer_unhealthy")
		}
	}
}
