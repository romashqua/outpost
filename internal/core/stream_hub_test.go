package core

import (
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	gatewayv1 "github.com/romashqua/outpost/pkg/pb/outpost/gateway/v1"
)

// mockSender implements streamSender for testing.
type mockSender struct {
	mu     sync.Mutex
	events []*gatewayv1.CoreEvent
	err    error // if set, Send returns this error
}

func (m *mockSender) Send(e *gatewayv1.CoreEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, e)
	return nil
}

func (m *mockSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func newTestHub() *StreamHub {
	return NewStreamHub(slog.Default())
}

func TestStreamHub_RegisterAndUnregister(t *testing.T) {
	hub := newTestHub()

	cleanup := hub.Register("gw-1", &mockSender{})
	if hub.ConnectedCount() != 1 {
		t.Errorf("expected 1 connected, got %d", hub.ConnectedCount())
	}

	cleanup()
	if hub.ConnectedCount() != 0 {
		t.Errorf("expected 0 connected after cleanup, got %d", hub.ConnectedCount())
	}
}

func TestStreamHub_Broadcast(t *testing.T) {
	hub := newTestHub()

	s1 := &mockSender{}
	s2 := &mockSender{}
	c1 := hub.Register("gw-1", s1)
	c2 := hub.Register("gw-2", s2)
	defer c1()
	defer c2()

	event := &gatewayv1.CoreEvent{}
	hub.Broadcast(event)

	if s1.count() != 1 {
		t.Errorf("gw-1: expected 1 event, got %d", s1.count())
	}
	if s2.count() != 1 {
		t.Errorf("gw-2: expected 1 event, got %d", s2.count())
	}
}

func TestStreamHub_BroadcastSkipsFailingSender(t *testing.T) {
	hub := newTestHub()

	good := &mockSender{}
	bad := &mockSender{err: errors.New("connection lost")}
	hub.Register("gw-good", good)
	hub.Register("gw-bad", bad)

	hub.Broadcast(&gatewayv1.CoreEvent{})

	// Good sender should still receive the event.
	if good.count() != 1 {
		t.Errorf("good sender: expected 1 event, got %d", good.count())
	}
}

func TestStreamHub_SendTo(t *testing.T) {
	hub := newTestHub()

	s1 := &mockSender{}
	s2 := &mockSender{}
	hub.Register("gw-1", s1)
	hub.Register("gw-2", s2)

	event := &gatewayv1.CoreEvent{}
	if err := hub.SendTo("gw-1", event); err != nil {
		t.Errorf("SendTo gw-1 should succeed: %v", err)
	}

	if s1.count() != 1 {
		t.Errorf("gw-1: expected 1 event, got %d", s1.count())
	}
	if s2.count() != 0 {
		t.Errorf("gw-2: expected 0 events, got %d", s2.count())
	}
}

func TestStreamHub_SendToDisconnectedGateway(t *testing.T) {
	hub := newTestHub()

	// SendTo a non-existent gateway should not error.
	if err := hub.SendTo("nonexistent", &gatewayv1.CoreEvent{}); err != nil {
		t.Errorf("SendTo nonexistent should return nil, got %v", err)
	}
}

func TestStreamHub_ConcurrentAccess(t *testing.T) {
	hub := newTestHub()
	var wg sync.WaitGroup
	var ops atomic.Int64

	// Concurrent register/broadcast/unregister.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			s := &mockSender{}
			cleanup := hub.Register("gw-"+string(rune('A'+id)), s)
			hub.Broadcast(&gatewayv1.CoreEvent{})
			ops.Add(1)
			cleanup()
		}(i)
	}
	wg.Wait()

	if ops.Load() != 20 {
		t.Errorf("expected 20 operations, got %d", ops.Load())
	}
}
