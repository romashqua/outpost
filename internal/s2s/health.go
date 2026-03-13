package s2s

import (
	"sync"
	"time"
)

// PeerHealth tracks the health status of an S2S tunnel peer.
type PeerHealth struct {
	GatewayID     string
	RemoteGateway string
	TunnelID      string
	IsHealthy     bool
	LatencyMs     int64
	LastCheck     time.Time
	LastHealthy   time.Time
	FailCount     int
}

// HealthTracker maintains health state for all S2S tunnel peers.
type HealthTracker struct {
	mu     sync.RWMutex
	peers  map[string]*PeerHealth // key: "tunnelID:gatewayID:remoteGatewayID"
	unhealthyThreshold int
}

func NewHealthTracker(unhealthyThreshold int) *HealthTracker {
	if unhealthyThreshold <= 0 {
		unhealthyThreshold = 3
	}
	return &HealthTracker{
		peers:              make(map[string]*PeerHealth),
		unhealthyThreshold: unhealthyThreshold,
	}
}

func (ht *HealthTracker) Report(tunnelID, gatewayID, remoteGatewayID string, isReachable bool, latencyMs int64) {
	key := tunnelID + ":" + gatewayID + ":" + remoteGatewayID

	ht.mu.Lock()
	defer ht.mu.Unlock()

	ph, ok := ht.peers[key]
	if !ok {
		ph = &PeerHealth{
			GatewayID:     gatewayID,
			RemoteGateway: remoteGatewayID,
			TunnelID:      tunnelID,
		}
		ht.peers[key] = ph
	}

	ph.LastCheck = time.Now()
	ph.LatencyMs = latencyMs

	if isReachable {
		ph.IsHealthy = true
		ph.LastHealthy = time.Now()
		ph.FailCount = 0
	} else {
		ph.FailCount++
		if ph.FailCount >= ht.unhealthyThreshold {
			ph.IsHealthy = false
		}
	}
}

func (ht *HealthTracker) GetHealth(tunnelID string) []PeerHealth {
	ht.mu.RLock()
	defer ht.mu.RUnlock()

	var result []PeerHealth
	for _, ph := range ht.peers {
		if ph.TunnelID == tunnelID {
			result = append(result, *ph)
		}
	}
	return result
}

func (ht *HealthTracker) IsHealthy(tunnelID, gatewayID, remoteGatewayID string) bool {
	key := tunnelID + ":" + gatewayID + ":" + remoteGatewayID

	ht.mu.RLock()
	defer ht.mu.RUnlock()

	ph, ok := ht.peers[key]
	if !ok {
		return false
	}
	return ph.IsHealthy
}
