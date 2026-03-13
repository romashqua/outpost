package s2s

// TopologyType represents the S2S network topology.
type TopologyType string

const (
	TopologyMesh     TopologyType = "mesh"
	TopologyHubSpoke TopologyType = "hub_spoke"
)

// Gateway represents a gateway participating in an S2S topology.
type Gateway struct {
	ID            string
	PublicKey     string
	Endpoint      string
	LocalSubnets  []string
	IsHub         bool
	ListenPort    int
}

// TunnelPeer represents a calculated WireGuard peer for an S2S tunnel.
type TunnelPeer struct {
	GatewayID    string
	PublicKey    string
	Endpoint     string
	AllowedIPs   []string
	Keepalive    int
}

// TunnelConfig is the computed WireGuard config for one gateway in an S2S tunnel.
type TunnelConfig struct {
	GatewayID     string
	InterfaceName string
	PrivateKey    string
	ListenPort    int
	LocalSubnets  []string
	Peers         []TunnelPeer
}

// CalculateTopology computes the WireGuard peer configurations for all gateways
// in the given topology. Each gateway receives a TunnelConfig describing
// what WireGuard interfaces and peers it needs.
func CalculateTopology(topology TopologyType, gateways []Gateway) []TunnelConfig {
	switch topology {
	case TopologyMesh:
		return calculateMesh(gateways)
	case TopologyHubSpoke:
		return calculateHubSpoke(gateways)
	default:
		return nil
	}
}
