package core

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

// hubPeerNotifier adapts StreamHub to the handler.PeerNotifier interface.
// After each peer change it also recomputes and pushes firewall rules
// to the affected gateways so that ACL enforcement stays in sync.
type hubPeerNotifier struct {
	hub    *StreamHub
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func (n *hubPeerNotifier) NotifyPeerAdd(pubkey string, allowedIPs []string) {
	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_ADD,
		Peer: &commonv1.Peer{
			PublicKey:           pubkey,
			AllowedIps:          allowedIPs,
			PersistentKeepalive: 25,
		},
	})

	// Recompute firewall rules for all gateways serving this device.
	n.refreshFirewallForPeer(pubkey)
}

func (n *hubPeerNotifier) NotifyPeerRemove(pubkey string) {
	// Capture gateway IDs before removing the peer (device may still exist).
	gatewayIDs := n.findGatewaysForPeer(pubkey)

	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_REMOVE,
		Peer: &commonv1.Peer{
			PublicKey: pubkey,
		},
	})

	// Recompute firewall rules for affected gateways.
	ctx := context.Background()
	for _, gwID := range gatewayIDs {
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
}

// refreshFirewallForPeer finds gateways that serve the device with the given
// pubkey and pushes updated firewall configs to them.
func (n *hubPeerNotifier) refreshFirewallForPeer(pubkey string) {
	gatewayIDs := n.findGatewaysForPeer(pubkey)
	ctx := context.Background()
	for _, gwID := range gatewayIDs {
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
}

// sendFirewallResult pushes a firewall config to the gateway and removes
// any WireGuard peers that are blocked by ZTNA policies.
func (n *hubPeerNotifier) sendFirewallResult(gatewayID string, result *firewallResult) {
	if result == nil {
		return
	}
	n.hub.SendFirewallUpdate(gatewayID, result.Config)
	n.removeZTNABlockedPeers(result.BlockedPubkeys)
}

// removeZTNABlockedPeers sends PeerUpdate ACTION_REMOVE for all ZTNA-blocked devices.
// This ensures immediate disconnection — the WireGuard peer is removed from the gateway,
// not just blocked by iptables (which has ESTABLISHED/RELATED bypass).
func (n *hubPeerNotifier) removeZTNABlockedPeers(pubkeys []string) {
	for _, pk := range pubkeys {
		n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
			Action: gatewayv1.PeerUpdate_ACTION_REMOVE,
			Peer:   &commonv1.Peer{PublicKey: pk},
		})
		preview := pk
		if len(preview) > 8 {
			preview = preview[:8] + "..."
		}
		n.logger.Info("ZTNA: removed WireGuard peer (policy violation)", "pubkey", preview)
	}
}

// findGatewaysForPeer returns gateway IDs that serve the network of the device
// with the given public key.
func (n *hubPeerNotifier) findGatewaysForPeer(pubkey string) []string {
	ctx := context.Background()
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT gn.gateway_id::text
		 FROM devices d
		 JOIN gateway_networks gn ON gn.network_id = d.network_id
		 WHERE d.wireguard_pubkey = $1`, pubkey)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// RefreshFirewallForUser recomputes and pushes firewall configs to all gateways
// serving networks that the given user has devices on.
func (n *hubPeerNotifier) RefreshFirewallForUser(userID string) {
	ctx := context.Background()
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT COALESCE(gn.gateway_id, g.id)::text
		 FROM devices d
		 LEFT JOIN gateway_networks gn ON gn.network_id = d.network_id
		 LEFT JOIN gateways g ON g.network_id = d.network_id
		 WHERE d.user_id::text = $1
		   AND d.is_approved = true
		   AND d.wireguard_pubkey != ''`, userID)
	if err != nil {
		n.logger.Warn("failed to find gateways for user", "user_id", userID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err != nil {
			continue
		}
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
}

// RefreshFirewallForGroup recomputes and pushes firewall configs to all gateways
// serving networks that have ACLs for the given group.
func (n *hubPeerNotifier) RefreshFirewallForGroup(groupID string) {
	ctx := context.Background()
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT COALESCE(gn.gateway_id, g.id)::text
		 FROM network_acls na
		 LEFT JOIN gateway_networks gn ON gn.network_id = na.network_id
		 LEFT JOIN gateways g ON g.network_id = na.network_id
		 WHERE na.group_id::text = $1`, groupID)
	if err != nil {
		n.logger.Warn("failed to find gateways for group", "group_id", groupID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err != nil {
			continue
		}
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
}

// RefreshFirewallForGateways recomputes and pushes firewall configs to the given gateways.
// Used when a network is deleted (gateway IDs captured before cascade delete).
func (n *hubPeerNotifier) RefreshFirewallForGateways(gatewayIDs []string) {
	ctx := context.Background()
	for _, gwID := range gatewayIDs {
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
}

// RefreshAllFirewalls recomputes and pushes firewall configs to ALL connected gateways.
// Used when a global policy (ZTNA) changes that may affect all devices.
func (n *hubPeerNotifier) RefreshAllFirewalls() {
	ctx := context.Background()
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT id::text FROM gateways WHERE is_active = true`)
	if err != nil {
		n.logger.Warn("failed to list gateways for global firewall refresh", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err != nil {
			continue
		}
		n.sendFirewallResult(gwID, buildFirewallConfigFromPool(ctx, n.pool, n.logger, gwID))
	}
	n.logger.Info("global firewall refresh triggered (ZTNA policy change)")
}

// NotifySmartRouteUpdate pushes updated smart route configs to all gateways
// serving networks associated with the given smart route.
func (n *hubPeerNotifier) NotifySmartRouteUpdate(smartRouteID string) {
	ctx := context.Background()
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT COALESCE(gn.gateway_id, g.id)::text
		 FROM network_smart_routes nsr
		 LEFT JOIN gateway_networks gn ON gn.network_id = nsr.network_id
		 LEFT JOIN gateways g ON g.network_id = nsr.network_id
		 WHERE nsr.smart_route_id::text = $1`, smartRouteID)
	if err != nil {
		n.logger.Warn("failed to find gateways for smart route", "smart_route_id", smartRouteID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err != nil {
			continue
		}
		srConfig := fetchSmartRoutesForGateway(ctx, n.pool, n.logger, gwID)
		n.hub.SendSmartRouteUpdate(gwID, srConfig)
	}
}

// hubGatewayNetworkNotifier handles gateway network change events by
// recomputing S2S routes and firewall rules for the affected gateway.
type hubGatewayNetworkNotifier struct {
	hub    *StreamHub
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func (n *hubGatewayNetworkNotifier) NotifyGatewayNetworksChanged(gatewayID string) {
	ctx := context.Background()

	// 1. Refresh firewall rules for the gateway itself.
	if result := buildFirewallConfigFromPool(ctx, n.pool, n.logger, gatewayID); result != nil {
		n.hub.SendFirewallUpdate(gatewayID, result.Config)
	}

	// 2. Refresh S2S configs for this gateway and all its S2S tunnel peers.
	// Find all tunnels this gateway participates in.
	rows, err := n.pool.Query(ctx,
		`SELECT DISTINCT m2.gateway_id::text
		 FROM s2s_tunnel_members m1
		 JOIN s2s_tunnel_members m2 ON m2.tunnel_id = m1.tunnel_id
		 JOIN s2s_tunnels t ON t.id = m1.tunnel_id
		 WHERE m1.gateway_id::text = $1 AND t.is_active = true`, gatewayID)
	if err != nil {
		n.logger.Warn("failed to find S2S peers for gateway network change", "gateway_id", gatewayID, "error", err)
		return
	}
	defer rows.Close()

	var peerGWs []string
	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err == nil {
			peerGWs = append(peerGWs, gwID)
		}
	}

	// Push updated S2S config to each affected gateway.
	s2sNotifier := &hubS2SNotifier{hub: n.hub, pool: n.pool}
	for _, gwID := range peerGWs {
		s2sNotifier.NotifyS2SUpdate(gwID, "", "network_change")
	}

	n.logger.Info("gateway networks changed, notified S2S peers", "gateway_id", gatewayID, "peer_count", len(peerGWs))
}

// hubS2SNotifier adapts StreamHub to the handler.S2SNotifier interface.
type hubS2SNotifier struct {
	hub  *StreamHub
	pool *pgxpool.Pool
}

func (n *hubS2SNotifier) NotifyS2SUpdate(gatewayID string, tunnelID string, action string) {
	// Send a full resync of S2S config to the gateway.
	ctx := context.Background()
	configs := n.buildS2SConfigs(ctx, gatewayID)

	for _, cfg := range configs {
		_ = n.hub.SendTo(gatewayID, &gatewayv1.CoreEvent{
			Event: &gatewayv1.CoreEvent_S2SUpdate{
				S2SUpdate: &gatewayv1.S2SUpdate{
					Action: gatewayv1.S2SUpdate_ACTION_ADD_TUNNEL,
					Tunnel: cfg,
				},
			},
		})
	}
}

func (n *hubS2SNotifier) buildS2SConfigs(ctx context.Context, gatewayID string) []*gatewayv1.S2STunnelConfig {
	rows, err := n.pool.Query(ctx,
		`SELECT t.id, t.name, t.topology, m.private_key, m.listen_port
		 FROM s2s_tunnels t
		 JOIN s2s_tunnel_members m ON m.tunnel_id = t.id
		 WHERE m.gateway_id::text = $1 AND t.is_active = true`, gatewayID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var configs []*gatewayv1.S2STunnelConfig
	for rows.Next() {
		var tunnelID, tunnelName, topology string
		var privateKey *string
		var listenPort int
		if err := rows.Scan(&tunnelID, &tunnelName, &topology, &privateKey, &listenPort); err != nil {
			continue
		}

		// Get all peers — use S2S-specific public keys (fall back to gateway's main key).
		peerRows, err := n.pool.Query(ctx,
			`SELECT m.gateway_id, COALESCE(m.public_key, g.wireguard_pubkey), g.endpoint,
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

		// Fetch manual routes and merge into peer AllowedIPs.
		routeRows, err := n.pool.Query(ctx,
			`SELECT destination::text, via_gateway::text FROM s2s_routes
			 WHERE tunnel_id = $1 AND is_active = true
			 ORDER BY metric ASC`, tunnelID)
		if err == nil {
			for routeRows.Next() {
				var dest, viaGw string
				if err := routeRows.Scan(&dest, &viaGw); err != nil {
					continue
				}
				for _, p := range peers {
					if p.GatewayId == viaGw {
						p.AllowedIps = append(p.AllowedIps, dest)
						break
					}
				}
			}
			routeRows.Close()
		}

		cfg := &gatewayv1.S2STunnelConfig{
			TunnelId:      tunnelID,
			InterfaceName: "wg-s2s-" + tunnelName,
			Peers:         peers,
			ListenPort:    int32(listenPort),
		}
		if privateKey != nil {
			cfg.PrivateKey = *privateKey
		}

		configs = append(configs, cfg)
	}

	return configs
}
