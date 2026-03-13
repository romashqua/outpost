package s2s

import "fmt"

// calculateMesh computes a full mesh topology where every gateway
// connects directly to every other gateway.
// Number of tunnels = n*(n-1)/2
func calculateMesh(gateways []Gateway) []TunnelConfig {
	configs := make([]TunnelConfig, 0, len(gateways))

	for i, gw := range gateways {
		var peers []TunnelPeer

		for j, remote := range gateways {
			if i == j {
				continue
			}

			// In mesh mode, each gateway allows all remote subnets.
			peers = append(peers, TunnelPeer{
				GatewayID:  remote.ID,
				PublicKey:  remote.PublicKey,
				Endpoint:   remote.Endpoint,
				AllowedIPs: remote.LocalSubnets,
				Keepalive:  25,
			})
		}

		configs = append(configs, TunnelConfig{
			GatewayID:     gw.ID,
			InterfaceName: fmt.Sprintf("wg-s2s-%d", i),
			ListenPort:    gw.ListenPort,
			LocalSubnets:  gw.LocalSubnets,
			Peers:         peers,
		})
	}

	return configs
}
