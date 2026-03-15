package core

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

// hubPeerNotifier adapts StreamHub to the handler.PeerNotifier interface.
type hubPeerNotifier struct {
	hub *StreamHub
}

func (n *hubPeerNotifier) NotifyPeerAdd(pubkey string, allowedIPs []string) {
	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_ADD,
		Peer: &commonv1.Peer{
			PublicKey:            pubkey,
			AllowedIps:          allowedIPs,
			PersistentKeepalive: 25,
		},
	})
}

func (n *hubPeerNotifier) NotifyPeerRemove(pubkey string) {
	n.hub.BroadcastPeerUpdate(&gatewayv1.PeerUpdate{
		Action: gatewayv1.PeerUpdate_ACTION_REMOVE,
		Peer: &commonv1.Peer{
			PublicKey: pubkey,
		},
	})
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
		`SELECT t.id, t.name, t.topology
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
		if err := rows.Scan(&tunnelID, &tunnelName, &topology); err != nil {
			continue
		}

		// Get all peers (other members of this tunnel).
		peerRows, err := n.pool.Query(ctx,
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
