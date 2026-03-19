package core

import (
	"context"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

const redisCoreEventsChannel = "outpost:core_events"

// streamSender abstracts the Send side of a gRPC server stream.
type streamSender interface {
	Send(*gatewayv1.CoreEvent) error
}

// StreamHub manages connected gateway streams and broadcasts events.
// When a Redis client is provided, events are published to a Pub/Sub channel
// so that other core instances can relay them to their locally connected gateways.
type StreamHub struct {
	mu      sync.RWMutex
	streams map[string]streamSender // keyed by gateway ID
	logger  *slog.Logger
	rdb     *redis.Client // nil when running single-core
	cancel  context.CancelFunc
}

// NewStreamHub creates a new StreamHub. Pass a non-nil Redis client to enable
// cross-core event propagation via Pub/Sub; pass nil for single-core mode
// (zero overhead — no goroutines, no Redis calls).
func NewStreamHub(logger *slog.Logger, rdb ...*redis.Client) *StreamHub {
	h := &StreamHub{
		streams: make(map[string]streamSender),
		logger:  logger,
	}
	if len(rdb) > 0 && rdb[0] != nil {
		h.rdb = rdb[0]
	}
	return h
}

// StartSubscriber launches the Redis Pub/Sub subscriber goroutine.
// Call this after NewStreamHub when Redis is configured. No-op if Redis is nil.
func (h *StreamHub) StartSubscriber(ctx context.Context) {
	if h.rdb == nil {
		return
	}
	subCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	go h.subscribe(subCtx)
}

// Stop cleans up the subscriber goroutine.
func (h *StreamHub) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
}

// subscribe listens on the Redis channel and forwards events to local gateways.
func (h *StreamHub) subscribe(ctx context.Context) {
	sub := h.rdb.Subscribe(ctx, redisCoreEventsChannel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event gatewayv1.CoreEvent
			if err := proto.Unmarshal([]byte(msg.Payload), &event); err != nil {
				h.logger.Warn("failed to unmarshal Redis event", "error", err)
				continue
			}
			// Forward to locally connected gateways only (no re-publish).
			h.broadcastLocal(&event)
		}
	}
}

// publish serializes and publishes an event to Redis Pub/Sub.
// No-op when Redis is not configured.
func (h *StreamHub) publish(event *gatewayv1.CoreEvent) {
	if h.rdb == nil {
		return
	}
	data, err := proto.Marshal(event)
	if err != nil {
		h.logger.Warn("failed to marshal event for Redis", "error", err)
		return
	}
	if err := h.rdb.Publish(context.Background(), redisCoreEventsChannel, data).Err(); err != nil {
		h.logger.Warn("failed to publish event to Redis", "error", err)
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

// broadcastLocal sends a CoreEvent to all locally connected gateways (no Redis publish).
func (h *StreamHub) broadcastLocal(event *gatewayv1.CoreEvent) {
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

// Broadcast sends a CoreEvent to all connected gateways on this core
// and publishes to Redis for other cores.
func (h *StreamHub) Broadcast(event *gatewayv1.CoreEvent) {
	h.broadcastLocal(event)
	h.publish(event)
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
		return nil // gateway not connected to this core
	}
	return s.Send(event)
}

// SendFirewallUpdate sends a FirewallUpdate event to a specific gateway.
// Also publishes to Redis so other cores can forward it.
func (h *StreamHub) SendFirewallUpdate(gatewayID string, config *commonv1.FirewallConfig) {
	event := &gatewayv1.CoreEvent{
		Event: &gatewayv1.CoreEvent_FirewallUpdate{
			FirewallUpdate: &gatewayv1.FirewallUpdate{
				Config: config,
			},
		},
	}
	if err := h.SendTo(gatewayID, event); err != nil {
		h.logger.Warn("failed to send firewall update", "gateway_id", gatewayID, "error", err)
	}
	h.publish(event)
}

// SendSmartRouteUpdate sends a SmartRouteUpdate event to a specific gateway.
// Also publishes to Redis so other cores can forward it.
func (h *StreamHub) SendSmartRouteUpdate(gatewayID string, config *commonv1.SmartRouteConfig) {
	event := &gatewayv1.CoreEvent{
		Event: &gatewayv1.CoreEvent_SmartRouteUpdate{
			SmartRouteUpdate: &gatewayv1.SmartRouteUpdate{
				Config: config,
			},
		},
	}
	if err := h.SendTo(gatewayID, event); err != nil {
		h.logger.Warn("failed to send smart route update", "gateway_id", gatewayID, "error", err)
	}
	h.publish(event)
}

// ConnectedCount returns the number of connected gateways.
func (h *StreamHub) ConnectedCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.streams)
}

// ensure BidiStreamingServer satisfies streamSender at compile time.
var _ streamSender = (grpc.BidiStreamingServer[gatewayv1.GatewayEvent, gatewayv1.CoreEvent])(nil)
