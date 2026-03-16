package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiter_AllowWithinLimit(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    3,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestIPRateLimiter_BlockExceedingLimit(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    2,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	rl.allow("1.2.3.4")
	rl.allow("1.2.3.4")

	if rl.allow("1.2.3.4") {
		t.Error("third request should be blocked")
	}
}

func TestIPRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    1,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	if !rl.allow("1.1.1.1") {
		t.Error("first IP first request should be allowed")
	}
	if !rl.allow("2.2.2.2") {
		t.Error("second IP first request should be allowed")
	}
	if rl.allow("1.1.1.1") {
		t.Error("first IP second request should be blocked")
	}
}

func TestIPRateLimiter_ExpiredEntriesCleared(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    1,
		window:   50 * time.Millisecond,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	if !rl.allow("1.2.3.4") {
		t.Error("first request should be allowed")
	}
	if rl.allow("1.2.3.4") {
		t.Error("second request should be blocked")
	}

	// Wait for window to expire.
	time.Sleep(60 * time.Millisecond)

	if !rl.allow("1.2.3.4") {
		t.Error("request after window expiry should be allowed")
	}
}

func TestRateLimitMiddleware_Returns429(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    1,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	handler := rateLimitMiddleware(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request — allowed.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", w.Code)
	}

	// Second request — rate limited.
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_StripsPort(t *testing.T) {
	rl := &ipRateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    1,
		window:   time.Minute,
		stop:     make(chan struct{}),
	}
	defer close(rl.stop)

	handler := rateLimitMiddleware(rl)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Same IP, different ports should be treated as same IP.
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "10.0.0.1:11111"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req1)

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "10.0.0.1:22222"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req2)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("same IP different port: expected 429, got %d", w.Code)
	}
}
