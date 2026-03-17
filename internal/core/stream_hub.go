package core

import (
	"log/slog"
	"sync"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
	"google.golang.org/grpc"
)

// streamSender abstracts the Send side of a gRPC server stream.
type streamSender interface {
	Send(*gatewayv1.CoreEvent) error
}

// StreamHub manages connected gateway streams and broadcasts events.
type StreamHub struct {
	mu      sync.RWMutex
	streams map[string]streamSender // keyed by gateway ID
	logger  *slog.Logger
}

// NewStreamHub creates a new StreamHub.
func NewStreamHub(logger *slog.Logger) *StreamHub {
	return &StreamHub{
		streams: make(map[string]streamSender),
		logger:  logger,
	}
}

// Register adds a gateway stream. Returns a cleanup function.
func (h *StreamHub) Register(gatewayID string, stream streamSender) func() {
	h.mu.Lock()
	h.streams[gatewayID] = stream
	h.mu.Unlock()
	h.logger.Info("gateway stream registered", "gateway_id", gatewayID)

	return func() {
		h.mu.Lock()
		delete(h.streams, gatewayID)
		h.mu.Unlock()
		h.logger.Info("gateway stream unregistered", "gateway_id", gatewayID)
	}
}

// Broadcast sends a CoreEvent to all connected gateways.
func (h *StreamHub) Broadcast(event *gatewayv1.CoreEvent) {
	h.mu.RLock()
	snapshot := make(map[string]streamSender, len(h.streams))
	for id, s := range h.streams {
		snapshot[id] = s
	}
	h.mu.RUnlock()

	for id, s := range snapshot {
		if err := s.Send(event); err != nil {
			h.logger.Warn("failed to send event to gateway", "gateway_id", id, "error", err)
		}
	}
}

// BroadcastPeerUpdate sends a PeerUpdate event to all connected gateways.
func (h *StreamHub) BroadcastPeerUpdate(update *gatewayv1.PeerUpdate) {
	h.Broadcast(&gatewayv1.CoreEvent{
		Event: &gatewayv1.CoreEvent_PeerUpdate{
			PeerUpdate: update,
		},
	})
}

// SendTo sends a CoreEvent to a specific gateway.
func (h *StreamHub) SendTo(gatewayID string, event *gatewayv1.CoreEvent) error {
	h.mu.RLock()
	s, ok := h.streams[gatewayID]
	h.mu.RUnlock()

	if !ok {
		return nil // gateway not connected
	}
	return s.Send(event)
}

// SendFirewallUpdate sends a FirewallUpdate event to a specific gateway.
func (h *StreamHub) SendFirewallUpdate(gatewayID string, config *commonv1.FirewallConfig) {
	_ = h.SendTo(gatewayID, &gatewayv1.CoreEvent{
		Event: &gatewayv1.CoreEvent_FirewallUpdate{
			FirewallUpdate: &gatewayv1.FirewallUpdate{
				Config: config,
			},
		},
	})
}

// SendSmartRouteUpdate sends a SmartRouteUpdate event to a specific gateway.
func (h *StreamHub) SendSmartRouteUpdate(gatewayID string, config *commonv1.SmartRouteConfig) {
	_ = h.SendTo(gatewayID, &gatewayv1.CoreEvent{
		Event: &gatewayv1.CoreEvent_SmartRouteUpdate{
			SmartRouteUpdate: &gatewayv1.SmartRouteUpdate{
				Config: config,
			},
		},
	})
}

// ConnectedCount returns the number of connected gateways.
func (h *StreamHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.streams)
}

// ensure BidiStreamingServer satisfies streamSender at compile time.
var _ streamSender = (grpc.BidiStreamingServer[gatewayv1.GatewayEvent, gatewayv1.CoreEvent])(nil)
