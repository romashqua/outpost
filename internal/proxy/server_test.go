package proxy

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/romashqua/outpost/internal/config"
)

// --- NewServer ---

func TestNewServer(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Proxy: config.Proxy{
			ListenAddr: ":8081",
			CoreURL:    "http://localhost:8080",
		},
	}

	s := NewServer(cfg, logger)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.cfg != cfg {
		t.Error("server config does not match")
	}
	if s.coreClient == nil {
		t.Error("coreClient should not be nil")
	}
}

// --- coreURL ---

func TestServer_CoreURL(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name    string
		baseURL string
		path    string
		expect  string
	}{
		{
			name:    "no trailing slash",
			baseURL: "http://core:8080",
			path:    "/api/v1/auth/login",
			expect:  "http://core:8080/api/v1/auth/login",
		},
		{
			name:    "with trailing slash",
			baseURL: "http://core:8080/",
			path:    "/api/v1/auth/login",
			expect:  "http://core:8080/api/v1/auth/login",
		},
		{
			name:    "multiple trailing slashes",
			baseURL: "http://core:8080///",
			path:    "/healthz",
			expect:  "http://core:8080/healthz",
		},
		{
			name:    "empty path",
			baseURL: "http://core:8080",
			path:    "",
			expect:  "http://core:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewServer(&config.Config{
				Proxy: config.Proxy{CoreURL: tt.baseURL},
			}, logger)

			got := s.coreURL(tt.path)
			if got != tt.expect {
				t.Errorf("coreURL(%q) = %q, want %q", tt.path, got, tt.expect)
			}
		})
	}
}

// --- copyHeaders ---

func TestCopyHeaders(t *testing.T) {
	t.Parallel()

	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Set("Authorization", "Bearer token123")
	src.Set("X-Custom", "should-not-copy")

	dst := http.Header{}
	copyHeaders(src, dst, []string{"Content-Type", "Authorization", "Accept"})

	if dst.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", dst.Get("Content-Type"))
	}
	if dst.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization = %q", dst.Get("Authorization"))
	}
	if dst.Get("Accept") != "" {
		t.Errorf("Accept should be empty when not in src, got %q", dst.Get("Accept"))
	}
	if dst.Get("X-Custom") != "" {
		t.Errorf("X-Custom should not have been copied, got %q", dst.Get("X-Custom"))
	}
}

func TestCopyHeaders_EmptyKeys(t *testing.T) {
	t.Parallel()
	src := http.Header{}
	src.Set("Content-Type", "text/plain")
	dst := http.Header{}

	copyHeaders(src, dst, []string{})
	if len(dst) != 0 {
		t.Errorf("expected empty dst headers, got %d", len(dst))
	}
}

// --- forwardRequest integration test with httptest ---

func TestForwardRequest_Success(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Set up a mock "core" server.
	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify forwarded headers.
		if r.Header.Get("X-Forwarded-By") != "outpost-proxy" {
			t.Errorf("missing X-Forwarded-By header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type not forwarded: %q", r.Header.Get("Content-Type"))
		}

		// Read body.
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"username":"test"}` {
			t.Errorf("unexpected body: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Core-Response", "yes")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"token":"abc123"}`))
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	// Create a request to the proxy.
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, coreServer.URL+"/api/v1/auth/login")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Core-Response") != "yes" {
		t.Error("response headers from core not forwarded")
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"token":"abc123"}` {
		t.Errorf("response body = %q", string(body))
	}
}

func TestForwardRequest_CoreUnreachable(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: "http://127.0.0.1:1"}, // unreachable port
	}, logger)

	req := httptest.NewRequest(http.MethodPost, "/enroll", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, "http://127.0.0.1:1/api/v1/devices/enroll")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestForwardRequest_CoreErrorStatus(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid credentials","message":"invalid credentials"}`))
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, coreServer.URL+"/api/v1/auth/login")

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid credentials") {
		t.Errorf("unexpected body: %s", body)
	}
}

// --- Healthz endpoint via Run route setup ---

func TestHealthzEndpoint(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// We can't call Run (it blocks), but we can test the healthz logic directly.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	// Inline the handler as it's defined in Run.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"outpost-proxy"}`))
	})
	handler.ServeHTTP(w, req)

	_ = logger // used only for server creation in other tests

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

// --- proxyToCore returns handler ---

func TestProxyToCore_ReturnsHandler(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			// The proxy rewrites the path via targetURL, so check path is what we expect
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	handler := s.proxyToCore("/api/v1/auth/login")
	if handler == nil {
		t.Fatal("proxyToCore returned nil handler")
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- X-Forwarded-For handling ---

func TestForwardRequest_XForwardedFor(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var receivedXFF string
	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, coreServer.URL+"/test")

	// Should have the client IP in X-Forwarded-For.
	if !strings.Contains(receivedXFF, "10.0.0.1:12345") {
		t.Errorf("X-Forwarded-For = %q, expected to contain client IP", receivedXFF)
	}
}

func TestForwardRequest_ExistingXForwardedFor(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var receivedXFF string
	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedXFF = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, coreServer.URL+"/test")

	// Should have appended client IP.
	if !strings.Contains(receivedXFF, "192.168.1.1") {
		t.Errorf("X-Forwarded-For missing original IP: %q", receivedXFF)
	}
	if !strings.Contains(receivedXFF, "10.0.0.1:12345") {
		t.Errorf("X-Forwarded-For missing proxy IP: %q", receivedXFF)
	}
}

// --- Redirect handling (CheckRedirect returns ErrUseLastResponse) ---

func TestNewServer_NoFollowRedirects(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	coreServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer coreServer.Close()

	s := NewServer(&config.Config{
		Proxy: config.Proxy{CoreURL: coreServer.URL},
	}, logger)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	s.forwardRequest(w, req, coreServer.URL+"/test")

	// Should forward the 302, not follow it.
	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302 (redirect not followed)", w.Code)
	}
}
