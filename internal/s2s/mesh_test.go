package s2s

import (
	"testing"
)

func TestCalculateMesh_TwoGateways(t *testing.T) {
	gateways := []Gateway{
		{ID: "gw-a", PublicKey: "keyA", Endpoint: "1.1.1.1:51820", LocalSubnets: []string{"10.0.1.0/24"}, ListenPort: 51821},
		{ID: "gw-b", PublicKey: "keyB", Endpoint: "2.2.2.2:51820", LocalSubnets: []string{"10.0.2.0/24"}, ListenPort: 51821},
	}

	configs := calculateMesh(gateways)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	// gw-a should have gw-b as peer.
	if len(configs[0].Peers) != 1 {
		t.Fatalf("gw-a: expected 1 peer, got %d", len(configs[0].Peers))
	}
	if configs[0].Peers[0].GatewayID != "gw-b" {
		t.Errorf("gw-a peer should be gw-b, got %s", configs[0].Peers[0].GatewayID)
	}

	// gw-b should have gw-a as peer.
	if len(configs[1].Peers) != 1 {
		t.Fatalf("gw-b: expected 1 peer, got %d", len(configs[1].Peers))
	}
	if configs[1].Peers[0].GatewayID != "gw-a" {
		t.Errorf("gw-b peer should be gw-a, got %s", configs[1].Peers[0].GatewayID)
	}
}

func TestCalculateMesh_ThreeGateways(t *testing.T) {
	gateways := []Gateway{
		{ID: "a", PublicKey: "ka", Endpoint: "1.1.1.1:51820", LocalSubnets: []string{"10.0.1.0/24"}, ListenPort: 51821},
		{ID: "b", PublicKey: "kb", Endpoint: "2.2.2.2:51820", LocalSubnets: []string{"10.0.2.0/24"}, ListenPort: 51822},
		{ID: "c", PublicKey: "kc", Endpoint: "3.3.3.3:51820", LocalSubnets: []string{"10.0.3.0/24"}, ListenPort: 51823},
	}

	configs := calculateMesh(gateways)
	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
	}

	// Each gateway should have 2 peers in full mesh.
	for _, cfg := range configs {
		if len(cfg.Peers) != 2 {
			t.Errorf("gateway %s: expected 2 peers, got %d", cfg.GatewayID, len(cfg.Peers))
		}
	}
}

func TestCalculateHubSpoke(t *testing.T) {
	gateways := []Gateway{
		{ID: "hub", PublicKey: "kh", Endpoint: "1.1.1.1:51820", LocalSubnets: []string{"10.0.0.0/24"}, IsHub: true, ListenPort: 51821},
		{ID: "spoke-a", PublicKey: "ka", Endpoint: "2.2.2.2:51820", LocalSubnets: []string{"10.0.1.0/24"}, ListenPort: 51822},
		{ID: "spoke-b", PublicKey: "kb", Endpoint: "3.3.3.3:51820", LocalSubnets: []string{"10.0.2.0/24"}, ListenPort: 51823},
	}

	configs := calculateHubSpoke(gateways)
	if len(configs) != 3 {
		t.Fatalf("expected 3 configs, got %d", len(configs))
	}

	// Hub should have 2 peers (both spokes).
	hubCfg := configs[0]
	if hubCfg.GatewayID != "hub" {
		t.Fatalf("first config should be hub, got %s", hubCfg.GatewayID)
	}
	if len(hubCfg.Peers) != 2 {
		t.Errorf("hub: expected 2 peers, got %d", len(hubCfg.Peers))
	}

	// Each spoke should have only 1 peer (the hub).
	for _, cfg := range configs[1:] {
		if len(cfg.Peers) != 1 {
			t.Errorf("spoke %s: expected 1 peer, got %d", cfg.GatewayID, len(cfg.Peers))
		}
		if cfg.Peers[0].GatewayID != "hub" {
			t.Errorf("spoke %s peer should be hub, got %s", cfg.GatewayID, cfg.Peers[0].GatewayID)
		}
	}
}

func TestCalculateTopology_InvalidType(t *testing.T) {
	configs := CalculateTopology("invalid", []Gateway{})
	if configs != nil {
		t.Errorf("expected nil for invalid topology, got %v", configs)
	}
}
