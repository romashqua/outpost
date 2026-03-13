package s2s

import "fmt"

// calculateHubSpoke computes a hub-and-spoke topology where all spokes
// connect through a designated hub gateway. The hub forwards traffic
// between spokes.
func calculateHubSpoke(gateways []Gateway) []TunnelConfig {
	var hub *Gateway
	var spokes []Gateway

	for i := range gateways {
		if gateways[i].IsHub {
			hub = &gateways[i]
		} else {
			spokes = append(spokes, gateways[i])
		}
	}

	if hub == nil {
		// No hub designated; fall back to first gateway.
		if len(gateways) == 0 {
			return nil
		}
		hub = &gateways[0]
		spokes = gateways[1:]
	}

	configs := make([]TunnelConfig, 0, len(gateways))

	// Hub config: peers with all spokes; allowed IPs include all spoke subnets.
	var hubPeers []TunnelPeer
	for _, spoke := range spokes {
		hubPeers = append(hubPeers, TunnelPeer{
			GatewayID:  spoke.ID,
			PublicKey:  spoke.PublicKey,
			Endpoint:   spoke.Endpoint,
			AllowedIPs: spoke.LocalSubnets,
			Keepalive:  25,
		})
	}
	configs = append(configs, TunnelConfig{
		GatewayID:     hub.ID,
		InterfaceName: "wg-s2s-hub",
		ListenPort:    hub.ListenPort,
		LocalSubnets:  hub.LocalSubnets,
		Peers:         hubPeers,
	})

	// Spoke configs: each spoke peers only with the hub.
	// AllowedIPs for the hub include all other spokes' subnets (transit routing).
	allSubnets := make([]string, 0)
	allSubnets = append(allSubnets, hub.LocalSubnets...)
	for _, spoke := range spokes {
		allSubnets = append(allSubnets, spoke.LocalSubnets...)
	}

	for i, spoke := range spokes {
		// Spoke allows hub's subnets + all other spoke subnets via hub.
		configs = append(configs, TunnelConfig{
			GatewayID:     spoke.ID,
			InterfaceName: fmt.Sprintf("wg-s2s-%d", i),
			ListenPort:    spoke.ListenPort,
			LocalSubnets:  spoke.LocalSubnets,
			Peers: []TunnelPeer{
				{
					GatewayID:  hub.ID,
					PublicKey:  hub.PublicKey,
					Endpoint:   hub.Endpoint,
					AllowedIPs: allSubnets,
					Keepalive:  25,
				},
			},
		})
	}

	return configs
}
