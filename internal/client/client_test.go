package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("https://vpn.example.com")

	if c.serverURL != "https://vpn.example.com" {
		t.Errorf("serverURL = %q, want %q", c.serverURL, "https://vpn.example.com")
	}
	if c.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 30s", c.httpClient.Timeout)
	}
	if c.token != "" {
		t.Errorf("token should be empty by default, got %q", c.token)
	}
	if c.configDir == "" {
		t.Error("configDir should not be empty")
	}
}

func TestLogin_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Username != "admin" || req.Password != "password123" {
			t.Errorf("unexpected credentials: %s / %s", req.Username, req.Password)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{
			Token:       "test-jwt-token",
			ExpiresAt:   time.Now().Add(24 * time.Hour).Unix(),
			MFARequired: false,
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()

	resp, err := c.Login(context.Background(), "admin", "password123")
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}

	if resp.Token != "test-jwt-token" {
		t.Errorf("Token = %q, want %q", resp.Token, "test-jwt-token")
	}
	if resp.MFARequired {
		t.Error("MFARequired should be false")
	}
	if c.token != "test-jwt-token" {
		t.Errorf("client token should be set to %q, got %q", "test-jwt-token", c.token)
	}
}

func TestLogin_MFARequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{
			Token:       "",
			MFARequired: true,
			MFAToken:    "mfa-challenge-token",
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()

	resp, err := c.Login(context.Background(), "user", "pass")
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}

	if !resp.MFARequired {
		t.Error("MFARequired should be true")
	}
	if resp.MFAToken != "mfa-challenge-token" {
		t.Errorf("MFAToken = %q, want %q", resp.MFAToken, "mfa-challenge-token")
	}
	// Token should NOT be set on the client when MFA is required.
	if c.token != "" {
		t.Errorf("client token should be empty when MFA required, got %q", c.token)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(apiError{Error: "invalid credentials"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()

	_, err := c.Login(context.Background(), "baduser", "badpass")
	if err == nil {
		t.Fatal("Login() should return error for invalid credentials")
	}

	if c.token != "" {
		t.Errorf("client token should be empty after failed login, got %q", c.token)
	}
}

func TestLogin_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()

	_, err := c.Login(context.Background(), "user", "pass")
	if err == nil {
		t.Fatal("Login() should return error for server error")
	}
}

func TestVerifyMFA_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/mfa/verify" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req mfaVerifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.MFAToken != "mfa-token" || req.Code != "123456" || req.Method != "totp" {
			http.Error(w, "invalid mfa", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mfaVerifyResponse{
			Token:     "full-session-token",
			ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
		})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()

	err := c.VerifyMFA(context.Background(), "mfa-token", "123456", "totp")
	if err != nil {
		t.Fatalf("VerifyMFA() error: %v", err)
	}

	if c.token != "full-session-token" {
		t.Errorf("client token = %q, want %q", c.token, "full-session-token")
	}
}

func TestLogin_UserAgentHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "outpost-client/1.0" {
			t.Errorf("User-Agent = %q, want %q", ua, "outpost-client/1.0")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(loginResponse{Token: "tok"})
	}))
	defer server.Close()

	c := NewClient(server.URL)
	c.configDir = t.TempDir()
	c.Login(context.Background(), "user", "pass")
}

func TestLogin_ContextCancellation(t *testing.T) {
	// Use an already-cancelled context to avoid any server blocking.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewClient("http://127.0.0.1:1") // address doesn't matter; context is already done
	c.configDir = t.TempDir()

	_, err := c.Login(ctx, "user", "pass")
	if err == nil {
		t.Error("Login() should return error when context is cancelled")
	}
}
