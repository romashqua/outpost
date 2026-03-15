package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/config"
	"github.com/romashqua/outpost/internal/core"
)

// testServer holds the shared httptest.Server and auth token for the E2E suite.
var (
	ts       *httptest.Server
	pool     *pgxpool.Pool
	adminJWT string
)

// TestMain bootstraps the test database, runs migrations, starts the HTTP
// server via httptest, and authenticates the seeded admin user. All subsequent
// tests share the same server instance.
func TestMain(m *testing.M) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		fmt.Println("TEST_DATABASE_URL not set, skipping E2E tests")
		os.Exit(0)
	}

	// Run database migrations.
	mig, err := migrate.New("file://../../migrations", dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create migrate instance: %v\n", err)
		os.Exit(1)
	}
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
	srcErr, dbErr := mig.Close()
	if srcErr != nil {
		fmt.Fprintf(os.Stderr, "migrate source close: %v\n", srcErr)
	}
	if dbErr != nil {
		fmt.Fprintf(os.Stderr, "migrate db close: %v\n", dbErr)
	}

	// Create pgx connection pool.
	ctx := context.Background()
	pool, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create connection pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Ensure the admin user exists (may have been deleted by previous test
	// runs). We use the same SQL as migration 000004 but with DO UPDATE to
	// reset the password in case it was changed.
	_, _ = pool.Exec(ctx,
		`INSERT INTO users (username, email, password_hash, first_name, last_name, is_active, is_admin)
		 VALUES ('admin', 'admin@outpost.local', crypt('admin', gen_salt('bf')), 'Admin', 'User', true, true)
		 ON CONFLICT (username) DO UPDATE SET
			password_hash = crypt('admin', gen_salt('bf')),
			is_active = true,
			is_admin = true`)

	// Ensure the default network exists.
	_, _ = pool.Exec(ctx,
		`INSERT INTO networks (name, address, dns, port)
		 VALUES ('default', '10.10.0.0/16', ARRAY['1.1.1.1', '8.8.8.8'], 51820)
		 ON CONFLICT (name) DO NOTHING`)

	// Build a minimal config for the server.
	cfg := &config.Config{}
	cfg.Auth.JWTSecret = "e2e-test-secret-key-change-me"
	cfg.Auth.SessionTTL = 24 * time.Hour
	cfg.Server.HTTPAddr = ":0"
	cfg.Server.GRPCAddr = ":0"
	cfg.OIDC.Issuer = "http://localhost"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := core.NewServer(cfg, pool, logger)
	router := srv.TestableRouter()

	ts = httptest.NewServer(router)
	defer ts.Close()

	code := m.Run()
	os.Exit(code)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// jsonBody marshals v to a JSON reader.
func jsonBody(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	return bytes.NewReader(b)
}

// doRequest creates and executes an HTTP request against the test server.
// It adds the Authorization header when token is non-empty.
func doRequest(t *testing.T, method, path string, body io.Reader, token string) *http.Response {
	t.Helper()
	url := ts.URL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, path, err)
	}
	return resp
}

// authRequest is a convenience wrapper: marshals body to JSON if non-nil,
// executes the request with the given token, and returns the response.
func authRequest(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = jsonBody(t, body)
	}
	return doRequest(t, method, path, reader, token)
}

// decodeJSON reads the response body and decodes it into dst.
func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatalf("failed to decode response JSON: %v\nbody: %s", err, string(b))
	}
}

// expectStatus asserts the HTTP status code.
func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d; body: %s", want, resp.StatusCode, string(b))
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHealthcheck(t *testing.T) {
	resp := doRequest(t, "GET", "/healthz", nil, "")
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doRequest(t, "GET", "/readyz", nil, "")
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestAuth(t *testing.T) {
	t.Run("login with valid credentials", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "admin",
		}
		resp := authRequest(t, "POST", "/api/v1/auth/login", body, "")
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		}
		decodeJSON(t, resp, &result)

		if result.Token == "" {
			t.Fatal("expected non-empty JWT token")
		}
		if result.ExpiresAt == 0 {
			t.Fatal("expected non-zero expires_at")
		}

		// Store for subsequent tests.
		adminJWT = result.Token
	})

	t.Run("login with wrong password", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
			"password": "wrongpassword",
		}
		resp := authRequest(t, "POST", "/api/v1/auth/login", body, "")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})

	t.Run("login with missing fields", func(t *testing.T) {
		body := map[string]string{
			"username": "admin",
		}
		resp := authRequest(t, "POST", "/api/v1/auth/login", body, "")
		expectStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("protected endpoint without token", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/users", nil, "")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})

	t.Run("protected endpoint with invalid token", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/users", nil, "invalid-token")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})

	t.Run("refresh token", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/auth/refresh", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Token     string `json:"token"`
			ExpiresAt int64  `json:"expires_at"`
		}
		decodeJSON(t, resp, &result)
		if result.Token == "" {
			t.Fatal("expected non-empty refreshed token")
		}
	})

	t.Run("logout", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/auth/logout", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

func TestUsers(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var createdUserID string

	t.Run("create user", func(t *testing.T) {
		body := map[string]any{
			"username":   "testuser1",
			"email":      "testuser1@outpost.local",
			"password":   "SecureP@ss123",
			"first_name": "Test",
			"last_name":  "User",
			"is_admin":   false,
		}
		resp := authRequest(t, "POST", "/api/v1/users", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var user map[string]any
		decodeJSON(t, resp, &user)
		id, ok := user["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected user id in response")
		}
		createdUserID = id

		if user["username"] != "testuser1" {
			t.Fatalf("expected username testuser1, got %v", user["username"])
		}
	})

	t.Run("create duplicate user returns 409", func(t *testing.T) {
		body := map[string]any{
			"username":   "testuser1",
			"email":      "testuser1@outpost.local",
			"password":   "AnotherP@ss",
			"first_name": "Dup",
			"last_name":  "User",
		}
		resp := authRequest(t, "POST", "/api/v1/users", body, adminJWT)
		expectStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})

	t.Run("list users includes new user", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Users []map[string]any `json:"users"`
		}
		decodeJSON(t, resp, &result)

		found := false
		for _, u := range result.Users {
			if u["id"] == createdUserID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("created user %s not found in user list", createdUserID)
		}
	})

	t.Run("get user by id", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users/"+createdUserID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var user map[string]any
		decodeJSON(t, resp, &user)
		if user["id"] != createdUserID {
			t.Fatalf("expected user id %s, got %v", createdUserID, user["id"])
		}
	})

	t.Run("get nonexistent user returns 404", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/users/00000000-0000-0000-0000-000000000000", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("update user", func(t *testing.T) {
		newFirst := "Updated"
		body := map[string]any{
			"first_name": newFirst,
		}
		resp := authRequest(t, "PUT", "/api/v1/users/"+createdUserID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var user map[string]any
		decodeJSON(t, resp, &user)
		if user["first_name"] != newFirst {
			t.Fatalf("expected first_name %q, got %v", newFirst, user["first_name"])
		}
	})

	t.Run("delete user", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/users/"+createdUserID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()

		// Verify it is gone.
		resp = authRequest(t, "GET", "/api/v1/users/"+createdUserID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

func TestNetworks(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var networkID string

	t.Run("create network", func(t *testing.T) {
		body := map[string]any{
			"name":      "e2e-test-net",
			"address":   "10.99.0.0/24",
			"dns":       []string{"1.1.1.1"},
			"port":      51821,
			"keepalive": 25,
		}
		resp := authRequest(t, "POST", "/api/v1/networks", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var net map[string]any
		decodeJSON(t, resp, &net)
		id, ok := net["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected network id in response")
		}
		networkID = id
	})

	t.Run("list networks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/networks", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var networks []map[string]any
		decodeJSON(t, resp, &networks)
		if len(networks) == 0 {
			t.Fatal("expected at least one network")
		}
	})

	t.Run("get network", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/networks/"+networkID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var net map[string]any
		decodeJSON(t, resp, &net)
		if net["id"] != networkID {
			t.Fatalf("expected network id %s, got %v", networkID, net["id"])
		}
	})

	t.Run("update network", func(t *testing.T) {
		newName := "e2e-test-net-updated"
		body := map[string]any{
			"name": newName,
		}
		resp := authRequest(t, "PUT", "/api/v1/networks/"+networkID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var net map[string]any
		decodeJSON(t, resp, &net)
		if net["name"] != newName {
			t.Fatalf("expected name %q, got %v", newName, net["name"])
		}
	})

	t.Run("delete network", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/networks/"+networkID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestDevices(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Get admin user ID for device creation.
	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	var deviceID string

	t.Run("create device", func(t *testing.T) {
		body := map[string]any{
			"name":             "e2e-test-device",
			"wireguard_pubkey": "auto-generated",
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var dev map[string]any
		decodeJSON(t, resp, &dev)
		id, ok := dev["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected device id in response")
		}
		deviceID = id
	})

	t.Run("list devices", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var devices []map[string]any
		decodeJSON(t, resp, &devices)
		if len(devices) == 0 {
			t.Fatal("expected at least one device")
		}
	})

	t.Run("get device", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var dev map[string]any
		decodeJSON(t, resp, &dev)
		if dev["id"] != deviceID {
			t.Fatalf("expected device id %s, got %v", deviceID, dev["id"])
		}
	})

	t.Run("approve device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/approve", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["status"] != "approved" {
			t.Fatalf("expected status approved, got %v", result["status"])
		}
	})

	t.Run("get device config", func(t *testing.T) {
		// Ensure there is an active gateway so the config endpoint can
		// construct a WireGuard configuration.
		var netID string
		err := pool.QueryRow(context.Background(),
			`SELECT id FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
		).Scan(&netID)
		if err != nil {
			t.Fatalf("no active network for config test: %v", err)
		}

		gwBody := map[string]any{
			"name":       "config-test-gw",
			"network_id": netID,
			"endpoint":   "gw.config-test.local:51820",
		}
		gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
		expectStatus(t, gwResp, http.StatusCreated)
		var gw map[string]any
		decodeJSON(t, gwResp, &gw)
		gwID := gw["id"].(string)

		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID+"/config", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)
		if cfg["config"] == nil || cfg["config"] == "" {
			t.Fatal("expected non-empty config in response")
		}
		if cfg["private_key"] == nil || cfg["private_key"] == "" {
			t.Fatal("expected non-empty private_key in response")
		}

		// Clean up the gateway.
		delResp := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
		expectStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()
	})

	t.Run("revoke device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/revoke", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["status"] != "revoked" {
			t.Fatalf("expected status revoked, got %v", result["status"])
		}
	})

	t.Run("delete device", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestGateways(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Get the default network ID (seeded by migration 000004).
	var networkID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&networkID)
	if err != nil {
		t.Fatalf("failed to get default network id: %v", err)
	}

	var gatewayID string

	t.Run("create gateway", func(t *testing.T) {
		body := map[string]any{
			"name":       "e2e-test-gw",
			"network_id": networkID,
			"endpoint":   "gw.e2e.test:51820",
		}
		resp := authRequest(t, "POST", "/api/v1/gateways", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var gw map[string]any
		decodeJSON(t, resp, &gw)
		id, ok := gw["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected gateway id in response")
		}
		gatewayID = id

		if gw["token"] == nil || gw["token"] == "" {
			t.Fatal("expected token in gateway creation response")
		}
	})

	t.Run("list gateways", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/gateways", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var gateways []map[string]any
		decodeJSON(t, resp, &gateways)
		if len(gateways) == 0 {
			t.Fatal("expected at least one gateway")
		}
	})

	t.Run("get gateway", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/gateways/"+gatewayID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var gw map[string]any
		decodeJSON(t, resp, &gw)
		if gw["id"] != gatewayID {
			t.Fatalf("expected gateway id %s, got %v", gatewayID, gw["id"])
		}
	})

	t.Run("delete gateway", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/gateways/"+gatewayID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestS2STunnels(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var tunnelID string

	t.Run("create tunnel with description", func(t *testing.T) {
		body := map[string]any{
			"name":        "e2e-mesh-tunnel",
			"topology":    "mesh",
			"description": "E2E test mesh tunnel",
		}
		resp := authRequest(t, "POST", "/api/v1/s2s-tunnels", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var tunnel map[string]any
		decodeJSON(t, resp, &tunnel)
		id, ok := tunnel["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected tunnel id in response")
		}
		tunnelID = id

		if tunnel["description"] != "E2E test mesh tunnel" {
			t.Fatalf("expected description 'E2E test mesh tunnel', got %v", tunnel["description"])
		}
	})

	t.Run("list tunnels", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var tunnels []map[string]any
		decodeJSON(t, resp, &tunnels)
		if len(tunnels) == 0 {
			t.Fatal("expected at least one tunnel")
		}
	})

	t.Run("get tunnel", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var tunnel map[string]any
		decodeJSON(t, resp, &tunnel)
		if tunnel["id"] != tunnelID {
			t.Fatalf("expected tunnel id %s, got %v", tunnelID, tunnel["id"])
		}
	})

	t.Run("delete tunnel", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestSmartRoutes(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var proxyServerID string
	var routeID string
	var entryID string

	t.Run("create proxy server", func(t *testing.T) {
		body := map[string]any{
			"name":    "e2e-socks5-proxy",
			"type":    "socks5",
			"address": "127.0.0.1",
			"port":    1080,
		}
		resp := authRequest(t, "POST", "/api/v1/smart-routes/proxy-servers", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var ps map[string]any
		decodeJSON(t, resp, &ps)
		id, ok := ps["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected proxy server id in response")
		}
		proxyServerID = id
	})

	t.Run("list proxy servers", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/smart-routes/proxy-servers", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var servers []map[string]any
		decodeJSON(t, resp, &servers)
		if len(servers) == 0 {
			t.Fatal("expected at least one proxy server")
		}
	})

	t.Run("create smart route", func(t *testing.T) {
		desc := "E2E test route group"
		body := map[string]any{
			"name":        "e2e-route-group",
			"description": desc,
		}
		resp := authRequest(t, "POST", "/api/v1/smart-routes", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var route map[string]any
		decodeJSON(t, resp, &route)
		id, ok := route["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected smart route id in response")
		}
		routeID = id
	})

	t.Run("add entry to route", func(t *testing.T) {
		body := map[string]any{
			"entry_type": "domain",
			"value":      "example.com",
			"action":     "proxy",
			"proxy_id":   proxyServerID,
			"priority":   10,
		}
		resp := authRequest(t, "POST", "/api/v1/smart-routes/"+routeID+"/entries", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var entry map[string]any
		decodeJSON(t, resp, &entry)
		id, ok := entry["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected entry id in response")
		}
		entryID = id
	})

	t.Run("get route with entries", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/smart-routes/"+routeID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var route map[string]any
		decodeJSON(t, resp, &route)
		entries, ok := route["entries"].([]any)
		if !ok || len(entries) == 0 {
			t.Fatal("expected at least one entry in route")
		}
	})

	t.Run("list smart routes", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/smart-routes", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var routes []map[string]any
		decodeJSON(t, resp, &routes)
		if len(routes) == 0 {
			t.Fatal("expected at least one smart route")
		}
	})

	t.Run("update smart route", func(t *testing.T) {
		newName := "e2e-route-group-updated"
		body := map[string]any{
			"name": newName,
		}
		resp := authRequest(t, "PUT", "/api/v1/smart-routes/"+routeID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var route map[string]any
		decodeJSON(t, resp, &route)
		if route["name"] != newName {
			t.Fatalf("expected name %q, got %v", newName, route["name"])
		}
	})

	t.Run("delete entry", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/smart-routes/"+routeID+"/entries/"+entryID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("delete smart route", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/smart-routes/"+routeID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("delete proxy server", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/smart-routes/proxy-servers/"+proxyServerID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestSettings(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("batch save settings", func(t *testing.T) {
		body := map[string]any{
			"vpn.mtu":           1420,
			"vpn.dns.enabled":   true,
			"general.site_name": "E2E Test Instance",
		}
		resp := authRequest(t, "PUT", "/api/v1/settings", body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["vpn.mtu"] == nil {
			t.Fatal("expected vpn.mtu in response")
		}
	})

	t.Run("get all settings", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/settings", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var settings map[string]any
		decodeJSON(t, resp, &settings)
		if settings["general.site_name"] == nil {
			t.Fatal("expected general.site_name to be present")
		}
	})

	t.Run("get single setting", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/settings/vpn.mtu", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var entry map[string]any
		decodeJSON(t, resp, &entry)
		if entry["key"] != "vpn.mtu" {
			t.Fatalf("expected key vpn.mtu, got %v", entry["key"])
		}
	})

	t.Run("delete setting", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/settings/vpn.mtu", nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestDashboard(t *testing.T) {
	t.Run("get dashboard stats", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/dashboard/stats", nil, "")
		expectStatus(t, resp, http.StatusOK)

		var stats map[string]any
		decodeJSON(t, resp, &stats)
		// Verify expected fields are present.
		for _, key := range []string{"active_users", "total_users", "active_devices", "total_devices"} {
			if _, ok := stats[key]; !ok {
				t.Fatalf("expected field %q in dashboard stats", key)
			}
		}
	})
}

func TestAnalytics(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("summary", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/analytics/summary", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var s map[string]any
		decodeJSON(t, resp, &s)
		for _, key := range []string{"total_rx_bytes", "total_tx_bytes", "total_flows"} {
			if _, ok := s[key]; !ok {
				t.Fatalf("expected field %q in analytics summary", key)
			}
		}
	})

	t.Run("bandwidth", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/analytics/bandwidth", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("top users", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/analytics/top-users", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("connections heatmap", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/analytics/connections-heatmap", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

func TestCompliance(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("full report", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/compliance/report", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("SOC2 checks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/compliance/soc2", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("ISO27001 checks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/compliance/iso27001", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("GDPR checks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/compliance/gdpr", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

func TestMFA(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("get MFA status", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/mfa/status", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("setup TOTP", func(t *testing.T) {
		body := map[string]string{
			"issuer": "Outpost VPN E2E",
		}
		resp := authRequest(t, "POST", "/api/v1/mfa/totp/setup", body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["secret"] == nil || result["secret"] == "" {
			t.Fatal("expected secret in TOTP setup response")
		}
		if result["qr_url"] == nil || result["qr_url"] == "" {
			t.Fatal("expected qr_url in TOTP setup response")
		}
	})

	t.Run("generate backup codes", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/mfa/backup-codes", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		codes, ok := result["codes"].([]any)
		if !ok || len(codes) == 0 {
			t.Fatal("expected non-empty backup codes in response")
		}
	})

	// Clean up: disable TOTP so it does not interfere with other tests.
	t.Run("disable TOTP", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/mfa/totp", nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestAudit(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("list audit logs", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/audit", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if _, ok := result["data"]; !ok {
			t.Fatal("expected 'data' field in audit log response")
		}
		if _, ok := result["total"]; !ok {
			t.Fatal("expected 'total' field in audit log response")
		}
	})

	t.Run("audit stats", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/audit/stats", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("export audit logs JSON", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/audit/export?format=json", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("export audit logs CSV", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/audit/export?format=csv", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		ct := resp.Header.Get("Content-Type")
		if ct != "text/csv" {
			t.Fatalf("expected Content-Type text/csv, got %s", ct)
		}
		resp.Body.Close()
	})
}

func TestWebhooks(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var webhookID string

	t.Run("create webhook", func(t *testing.T) {
		body := map[string]any{
			"url":    "https://httpbin.org/post",
			"secret": "e2e-test-secret",
			"events": []string{"*"},
		}
		resp := authRequest(t, "POST", "/api/v1/webhooks", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var wh map[string]any
		decodeJSON(t, resp, &wh)
		id, ok := wh["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected webhook id in response")
		}
		webhookID = id
	})

	t.Run("list webhooks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/webhooks", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var webhooks []map[string]any
		decodeJSON(t, resp, &webhooks)
		if len(webhooks) == 0 {
			t.Fatal("expected at least one webhook")
		}
	})

	t.Run("get webhook", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/webhooks/"+webhookID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var wh map[string]any
		decodeJSON(t, resp, &wh)
		if wh["id"] != webhookID {
			t.Fatalf("expected webhook id %s, got %v", webhookID, wh["id"])
		}
	})

	t.Run("delete webhook", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/webhooks/"+webhookID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestUserActivate(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create a deactivated user, then activate.
	body := map[string]any{
		"username":   "activatetest",
		"email":      "activatetest@outpost.local",
		"password":   "P@ssword123",
		"first_name": "Activate",
		"last_name":  "Test",
	}
	resp := authRequest(t, "POST", "/api/v1/users", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)

	var user map[string]any
	decodeJSON(t, resp, &user)
	userID := user["id"].(string)

	// Deactivate via update.
	deactivate := map[string]any{"is_active": false}
	resp = authRequest(t, "PUT", "/api/v1/users/"+userID, deactivate, adminJWT)
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Activate via PATCH endpoint.
	resp = authRequest(t, "PATCH", "/api/v1/users/"+userID+"/activate", nil, adminJWT)
	expectStatus(t, resp, http.StatusOK)

	decodeJSON(t, resp, &user)
	if user["is_active"] != true {
		t.Fatalf("expected is_active=true after activation, got %v", user["is_active"])
	}

	// Clean up.
	resp = authRequest(t, "DELETE", "/api/v1/users/"+userID, nil, adminJWT)
	expectStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestLastAdminProtection(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Try to delete the sole admin — should fail with 409.
	var adminID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID)
	if err != nil {
		t.Fatalf("failed to get admin id: %v", err)
	}

	resp := authRequest(t, "DELETE", "/api/v1/users/"+adminID, nil, adminJWT)
	expectStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
}

func TestOpenAPISpec(t *testing.T) {
	resp := doRequest(t, "GET", "/api/docs/openapi.yaml", nil, "")
	expectStatus(t, resp, http.StatusOK)
	ct := resp.Header.Get("Content-Type")
	if ct != "application/yaml" {
		t.Fatalf("expected Content-Type application/yaml, got %s", ct)
	}
	resp.Body.Close()
}

func TestMetrics(t *testing.T) {
	resp := doRequest(t, "GET", "/metrics", nil, "")
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestGroups(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var groupID string

	t.Run("create group", func(t *testing.T) {
		body := map[string]any{
			"name":        "e2e-test-group",
			"description": "Group for E2E tests",
		}
		resp := authRequest(t, "POST", "/api/v1/groups", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var g map[string]any
		decodeJSON(t, resp, &g)
		id, ok := g["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected group id in response")
		}
		groupID = id
	})

	t.Run("list groups", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/groups", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var groups []map[string]any
		decodeJSON(t, resp, &groups)
		if len(groups) == 0 {
			t.Fatal("expected at least one group")
		}
	})

	t.Run("get group", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/groups/"+groupID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var g map[string]any
		decodeJSON(t, resp, &g)
		if g["id"] != groupID {
			t.Fatalf("expected group id %s, got %v", groupID, g["id"])
		}
	})

	t.Run("update group", func(t *testing.T) {
		body := map[string]any{
			"name": "e2e-test-group-updated",
		}
		resp := authRequest(t, "PUT", "/api/v1/groups/"+groupID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var g map[string]any
		decodeJSON(t, resp, &g)
		if g["name"] != "e2e-test-group-updated" {
			t.Fatalf("expected updated name, got %v", g["name"])
		}
	})

	t.Run("add member to group", func(t *testing.T) {
		var adminID string
		err := pool.QueryRow(context.Background(),
			`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminID)
		if err != nil {
			t.Fatalf("failed to get admin id: %v", err)
		}

		body := map[string]any{
			"user_id": adminID,
		}
		resp := authRequest(t, "POST", "/api/v1/groups/"+groupID+"/members", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	})

	t.Run("list group members", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/groups/"+groupID+"/members", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var members []map[string]any
		decodeJSON(t, resp, &members)
		if len(members) == 0 {
			t.Fatal("expected at least one member")
		}
	})

	t.Run("add ACL to group", func(t *testing.T) {
		var netID string
		err := pool.QueryRow(context.Background(),
			`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)
		if err != nil {
			t.Fatalf("failed to get default network: %v", err)
		}

		body := map[string]any{
			"network_id":  netID,
			"allowed_ips": []string{"10.0.0.0/8"},
		}
		resp := authRequest(t, "POST", "/api/v1/groups/"+groupID+"/acls", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	})

	t.Run("list group ACLs", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/groups/"+groupID+"/acls", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var acls []map[string]any
		decodeJSON(t, resp, &acls)
		if len(acls) == 0 {
			t.Fatal("expected at least one ACL")
		}
	})

	t.Run("delete group", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/groups/"+groupID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestDeviceEnroll(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Clean up any leftover device from previous runs.
	_, _ = pool.Exec(context.Background(),
		`DELETE FROM devices WHERE name = 'enroll-test-device' OR wireguard_pubkey = 'dGVzdHB1YmtleQ==AAAAAAAAAAAAAAAAAAAAAAAA'`)

	// Ensure a gateway exists for enrollment.
	var netID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`).Scan(&netID)
	if err != nil {
		t.Fatalf("no active network: %v", err)
	}

	gwBody := map[string]any{
		"name":       "enroll-test-gw",
		"network_id": netID,
		"endpoint":   "gw.enroll-test.local:51820",
	}
	gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
	expectStatus(t, gwResp, http.StatusCreated)
	var gw map[string]any
	decodeJSON(t, gwResp, &gw)
	gwID := gw["id"].(string)

	t.Run("enroll device", func(t *testing.T) {
		body := map[string]any{
			"name":             "enroll-test-device",
			"wireguard_pubkey": "dGVzdHB1YmtleQ==AAAAAAAAAAAAAAAAAAAAAAAA",
		}
		resp := authRequest(t, "POST", "/api/v1/devices/enroll", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["device_id"] == nil || result["device_id"] == "" {
			t.Fatal("expected device_id in enroll response")
		}
		if result["address"] == nil || result["address"] == "" {
			t.Fatal("expected address in enroll response")
		}
		if result["endpoint"] == nil || result["endpoint"] == "" {
			t.Fatal("expected endpoint in enroll response")
		}
		if result["server_public_key"] == nil || result["server_public_key"] == "" {
			t.Fatal("expected server_public_key in enroll response")
		}
	})

	// Clean up gateway.
	delResp := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
	expectStatus(t, delResp, http.StatusNoContent)
	delResp.Body.Close()
}

func TestZTNA(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("get trust config", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/ztna/trust-config", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)
		if cfg["threshold_high"] == nil {
			t.Fatal("expected threshold_high in trust config")
		}
	})

	t.Run("update trust config", func(t *testing.T) {
		body := map[string]any{
			"weight_disk_encryption":     20,
			"weight_screen_lock":         15,
			"weight_antivirus":           15,
			"weight_firewall":            15,
			"weight_os_version":          20,
			"weight_mfa":                 15,
			"threshold_high":             80,
			"threshold_medium":           50,
			"threshold_low":              20,
			"auto_restrict_below_medium": true,
			"auto_block_below_low":       false,
		}
		resp := authRequest(t, "PUT", "/api/v1/ztna/trust-config", body, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	var policyID string

	t.Run("create ZTNA policy", func(t *testing.T) {
		desc := "E2E test ZTNA policy"
		body := map[string]any{
			"name":        "e2e-test-policy",
			"description": &desc,
			"action":      "restrict",
			"conditions":  map[string]any{"min_trust_score": 50},
			"network_ids": []string{},
			"priority":    10,
		}
		resp := authRequest(t, "POST", "/api/v1/ztna/policies", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var p map[string]any
		decodeJSON(t, resp, &p)
		id, ok := p["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected policy id in response")
		}
		policyID = id
	})

	t.Run("list ZTNA policies", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/ztna/policies", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var policies []map[string]any
		decodeJSON(t, resp, &policies)
		if len(policies) == 0 {
			t.Fatal("expected at least one ZTNA policy")
		}
	})

	t.Run("delete ZTNA policy", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/ztna/policies/"+policyID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("get trust score for admin device", func(t *testing.T) {
		// This may return 404 if no device exists — that's OK.
		var deviceID string
		err := pool.QueryRow(context.Background(),
			`SELECT id FROM devices LIMIT 1`).Scan(&deviceID)
		if err != nil {
			t.Skip("no device to test trust score")
		}

		resp := authRequest(t, "GET", "/api/v1/ztna/trust-score/"+deviceID, nil, adminJWT)
		// Accept 200 or 404 (no score computed yet).
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 200 or 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

func TestNATTraversal(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("list relay servers", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/nat/relays", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var relays []map[string]any
		decodeJSON(t, resp, &relays)
		// Empty list is OK — just ensure endpoint works.
	})

	var relayID string

	t.Run("create relay server", func(t *testing.T) {
		body := map[string]any{
			"name":     "e2e-stun-relay",
			"address":  "stun.e2e.local:3478",
			"region":   "eu-west",
			"protocol": "stun",
		}
		resp := authRequest(t, "POST", "/api/v1/nat/relays", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var relay map[string]any
		decodeJSON(t, resp, &relay)
		id, ok := relay["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected relay id in response")
		}
		relayID = id
	})

	t.Run("delete relay server", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/nat/relays/"+relayID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	t.Run("get NAT status for device", func(t *testing.T) {
		var deviceID string
		err := pool.QueryRow(context.Background(),
			`SELECT id FROM devices LIMIT 1`).Scan(&deviceID)
		if err != nil {
			t.Skip("no device to test NAT status")
		}

		resp := authRequest(t, "GET", "/api/v1/nat/status/"+deviceID, nil, adminJWT)
		// Accept 200 or 404 (no NAT check done yet).
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 200 or 404, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

func TestTenants(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Clean up any leftover tenant from previous runs.
	_, _ = pool.Exec(context.Background(),
		`DELETE FROM tenants WHERE slug = 'e2e-test-org-v2'`)

	var tenantID string

	t.Run("create tenant", func(t *testing.T) {
		body := map[string]any{
			"name":        "E2E Test Org",
			"slug":        "e2e-test-org-v2",
			"plan":        "pro",
			"max_users":   100,
			"max_devices": 200,
		}
		resp := authRequest(t, "POST", "/api/v1/tenants", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var tenant map[string]any
		decodeJSON(t, resp, &tenant)
		id, ok := tenant["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected tenant id in response")
		}
		tenantID = id
	})

	t.Run("list tenants", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/tenants", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var tenants []map[string]any
		decodeJSON(t, resp, &tenants)
		if len(tenants) == 0 {
			t.Fatal("expected at least one tenant")
		}
	})

	t.Run("get tenant", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/tenants/"+tenantID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var tenant map[string]any
		decodeJSON(t, resp, &tenant)
		if tenant["id"] != tenantID {
			t.Fatalf("expected tenant id %s, got %v", tenantID, tenant["id"])
		}
	})

	t.Run("get tenant stats", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/tenants/"+tenantID+"/stats", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var stats map[string]any
		decodeJSON(t, resp, &stats)
		if stats["tenant_id"] == nil {
			t.Fatal("expected tenant_id in stats response")
		}
	})

	t.Run("update tenant", func(t *testing.T) {
		body := map[string]any{
			"name": "E2E Test Org Updated",
		}
		resp := authRequest(t, "PUT", "/api/v1/tenants/"+tenantID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var tenant map[string]any
		decodeJSON(t, resp, &tenant)
		if tenant["name"] != "E2E Test Org Updated" {
			t.Fatalf("expected updated name, got %v", tenant["name"])
		}
	})

	t.Run("deactivate tenant", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/tenants/"+tenantID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestS2SMembers(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create a tunnel and gateway to test members.
	tunnelBody := map[string]any{
		"name":        "e2e-member-test",
		"topology":    "mesh",
		"description": "Testing members",
	}
	tunnelResp := authRequest(t, "POST", "/api/v1/s2s-tunnels", tunnelBody, adminJWT)
	expectStatus(t, tunnelResp, http.StatusCreated)
	var tunnel map[string]any
	decodeJSON(t, tunnelResp, &tunnel)
	tunnelID := tunnel["id"].(string)

	var netID string
	_ = pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)

	gwBody := map[string]any{
		"name":       "s2s-member-gw",
		"network_id": netID,
		"endpoint":   "gw.s2s-member.test:51820",
	}
	gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
	expectStatus(t, gwResp, http.StatusCreated)
	var gw map[string]any
	decodeJSON(t, gwResp, &gw)
	gwID := gw["id"].(string)

	t.Run("add member", func(t *testing.T) {
		body := map[string]any{
			"gateway_id":    gwID,
			"local_subnets": []string{"192.168.1.0/24"},
		}
		resp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/members", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	})

	t.Run("list members", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels/"+tunnelID+"/members", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var members []map[string]any
		decodeJSON(t, resp, &members)
		if len(members) == 0 {
			t.Fatal("expected at least one member")
		}
	})

	t.Run("list routes", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels/"+tunnelID+"/routes", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("remove member", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID+"/members/"+gwID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	// Clean up.
	delResp := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
	expectStatus(t, delResp, http.StatusNoContent)
	delResp.Body.Close()

	delGw := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
	expectStatus(t, delGw, http.StatusNoContent)
	delGw.Body.Close()
}

// ---------------------------------------------------------------------------
// Additional E2E tests for previously uncovered endpoints
// ---------------------------------------------------------------------------

func TestSessions(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("list sessions", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/sessions", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var sessions []map[string]any
		decodeJSON(t, resp, &sessions)
		// Current admin session should be present.
		if len(sessions) == 0 {
			t.Fatal("expected at least one session (the current admin session)")
		}
	})

	t.Run("delete nonexistent session returns 404", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/sessions/00000000-0000-0000-0000-000000000000", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

func TestS2SRoutes(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create tunnel and gateway for route testing.
	tunnelBody := map[string]any{
		"name":        "e2e-route-test",
		"topology":    "mesh",
		"description": "Testing S2S routes",
	}
	tunnelResp := authRequest(t, "POST", "/api/v1/s2s-tunnels", tunnelBody, adminJWT)
	expectStatus(t, tunnelResp, http.StatusCreated)
	var tunnel map[string]any
	decodeJSON(t, tunnelResp, &tunnel)
	tunnelID := tunnel["id"].(string)

	var netID string
	_ = pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)

	gwBody := map[string]any{
		"name":       "s2s-route-gw",
		"network_id": netID,
		"endpoint":   "gw.s2s-route.test:51820",
	}
	gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
	expectStatus(t, gwResp, http.StatusCreated)
	var gw map[string]any
	decodeJSON(t, gwResp, &gw)
	gwID := gw["id"].(string)

	// Add gateway as member first.
	memberBody := map[string]any{
		"gateway_id":    gwID,
		"local_subnets": []string{"172.16.0.0/24"},
	}
	memberResp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/members", memberBody, adminJWT)
	expectStatus(t, memberResp, http.StatusCreated)
	memberResp.Body.Close()

	var routeID string

	t.Run("add route", func(t *testing.T) {
		body := map[string]any{
			"destination": "10.20.0.0/16",
			"via_gateway": gwID,
			"metric":      50,
		}
		resp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/routes", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var route map[string]any
		decodeJSON(t, resp, &route)
		id, ok := route["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected route id in response")
		}
		routeID = id
	})

	t.Run("list routes includes new route", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels/"+tunnelID+"/routes", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var routes []map[string]any
		decodeJSON(t, resp, &routes)
		if len(routes) == 0 {
			t.Fatal("expected at least one route")
		}
	})

	t.Run("delete route", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/s2s-tunnels/"+tunnelID+"/routes/"+routeID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	// Clean up.
	delTunnel := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
	expectStatus(t, delTunnel, http.StatusNoContent)
	delTunnel.Body.Close()

	delGw := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
	expectStatus(t, delGw, http.StatusNoContent)
	delGw.Body.Close()
}

func TestS2SConfigGeneration(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create tunnel and gateway for config generation.
	tunnelBody := map[string]any{
		"name":        "e2e-config-gen",
		"topology":    "mesh",
		"description": "Testing config generation",
	}
	tunnelResp := authRequest(t, "POST", "/api/v1/s2s-tunnels", tunnelBody, adminJWT)
	expectStatus(t, tunnelResp, http.StatusCreated)
	var tunnel map[string]any
	decodeJSON(t, tunnelResp, &tunnel)
	tunnelID := tunnel["id"].(string)

	var netID string
	_ = pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)

	gwBody := map[string]any{
		"name":       "s2s-config-gw",
		"network_id": netID,
		"endpoint":   "gw.s2s-config.test:51820",
	}
	gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
	expectStatus(t, gwResp, http.StatusCreated)
	var gw map[string]any
	decodeJSON(t, gwResp, &gw)
	gwID := gw["id"].(string)

	// Add gateway as member.
	memberBody := map[string]any{
		"gateway_id":    gwID,
		"local_subnets": []string{"192.168.10.0/24"},
	}
	memberResp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/members", memberBody, adminJWT)
	expectStatus(t, memberResp, http.StatusCreated)
	memberResp.Body.Close()

	t.Run("generate config for gateway member", func(t *testing.T) {
		resp := authRequest(t, "GET",
			"/api/v1/s2s-tunnels/"+tunnelID+"/config/"+gwID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)
		if cfg["config"] == nil || cfg["config"] == "" {
			t.Fatal("expected non-empty config in response")
		}
	})

	t.Run("generate config for non-member returns 404", func(t *testing.T) {
		resp := authRequest(t, "GET",
			"/api/v1/s2s-tunnels/"+tunnelID+"/config/00000000-0000-0000-0000-000000000000", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	// Clean up.
	delTunnel := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
	expectStatus(t, delTunnel, http.StatusNoContent)
	delTunnel.Body.Close()

	delGw := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
	expectStatus(t, delGw, http.StatusNoContent)
	delGw.Body.Close()
}

func TestS2SAllowedDomains(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create tunnel for domain testing.
	tunnelBody := map[string]any{
		"name":        "e2e-domain-test",
		"topology":    "mesh",
		"description": "Testing S2S allowed domains",
	}
	tunnelResp := authRequest(t, "POST", "/api/v1/s2s-tunnels", tunnelBody, adminJWT)
	expectStatus(t, tunnelResp, http.StatusCreated)
	var tunnel map[string]any
	decodeJSON(t, tunnelResp, &tunnel)
	tunnelID := tunnel["id"].(string)

	var domainID string

	t.Run("add domain", func(t *testing.T) {
		body := map[string]any{
			"domain": "*.example.com",
		}
		resp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/domains", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var d map[string]any
		decodeJSON(t, resp, &d)
		id, ok := d["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected domain id in response")
		}
		domainID = id

		if d["domain"] != "*.example.com" {
			t.Fatalf("expected domain *.example.com, got %v", d["domain"])
		}
	})

	t.Run("list domains", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/s2s-tunnels/"+tunnelID+"/domains", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var domains []map[string]any
		decodeJSON(t, resp, &domains)
		if len(domains) == 0 {
			t.Fatal("expected at least one domain")
		}
	})

	t.Run("add duplicate domain returns 409", func(t *testing.T) {
		body := map[string]any{
			"domain": "*.example.com",
		}
		resp := authRequest(t, "POST", "/api/v1/s2s-tunnels/"+tunnelID+"/domains", body, adminJWT)
		expectStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})

	t.Run("delete domain", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/s2s-tunnels/"+tunnelID+"/domains/"+domainID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	// Clean up.
	delTunnel := authRequest(t, "DELETE", "/api/v1/s2s-tunnels/"+tunnelID, nil, adminJWT)
	expectStatus(t, delTunnel, http.StatusNoContent)
	delTunnel.Body.Close()
}

func TestSCIM(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Set up a SCIM bearer token in the settings table so we can authenticate.
	scimToken := "e2e-scim-test-token"
	_, err := pool.Exec(context.Background(),
		`INSERT INTO settings (key, value) VALUES ('scim_token', $1)
		 ON CONFLICT (key) DO UPDATE SET value = $1`,
		fmt.Sprintf(`"%s"`, scimToken))
	if err != nil {
		t.Fatalf("failed to set SCIM token: %v", err)
	}

	t.Run("ServiceProviderConfig", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/scim/v2/ServiceProviderConfig", nil, scimToken)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)
		if cfg["schemas"] == nil {
			t.Fatal("expected schemas field in ServiceProviderConfig")
		}
	})

	t.Run("Schemas", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/scim/v2/Schemas", nil, scimToken)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("list SCIM users", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/scim/v2/Users", nil, scimToken)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["schemas"] == nil {
			t.Fatal("expected schemas in SCIM Users response")
		}
	})

	t.Run("list SCIM groups", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/scim/v2/Groups", nil, scimToken)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["schemas"] == nil {
			t.Fatal("expected schemas in SCIM Groups response")
		}
	})

	t.Run("SCIM with invalid token returns 401", func(t *testing.T) {
		resp := doRequest(t, "GET", "/api/v1/scim/v2/Users", nil, "wrong-token")
		expectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})

	// Clean up the SCIM token.
	_, _ = pool.Exec(context.Background(),
		`DELETE FROM settings WHERE key = 'scim_token'`)
}

func TestMailTest(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("test SMTP without config", func(t *testing.T) {
		body := map[string]any{
			"to": "test@outpost.local",
		}
		resp := authRequest(t, "POST", "/api/v1/mail/test", body, adminJWT)
		// Without SMTP configured, expect an error response (422 or 400),
		// but the route should exist and not return 404.
		if resp.StatusCode == http.StatusNotFound {
			t.Fatal("expected mail/test endpoint to exist, got 404")
		}
		resp.Body.Close()
	})
}

func TestWebAuthnCredentials(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("list WebAuthn credentials", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/mfa/webauthn/credentials", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var creds []map[string]any
		decodeJSON(t, resp, &creds)
		// Empty list is fine — just verify the endpoint works.
	})

	t.Run("delete nonexistent credential returns 404", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/mfa/webauthn/credentials/00000000-0000-0000-0000-000000000000", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

func TestZTNAPosture(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create a device to report posture for.
	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	devBody := map[string]any{
		"name":             "posture-test-device",
		"wireguard_pubkey": "auto-generated",
		"user_id":          adminUserID,
	}
	devResp := authRequest(t, "POST", "/api/v1/devices", devBody, adminJWT)
	expectStatus(t, devResp, http.StatusCreated)
	var dev map[string]any
	decodeJSON(t, devResp, &dev)
	deviceID := dev["id"].(string)

	t.Run("report posture", func(t *testing.T) {
		body := map[string]any{
			"device_id":          deviceID,
			"os_type":            "linux",
			"os_version":         "6.1.0",
			"disk_encrypted":     true,
			"screen_lock_enabled": true,
			"antivirus_active":   false,
			"firewall_enabled":   true,
		}
		resp := authRequest(t, "POST", "/api/v1/ztna/posture", body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if result["trust_score"] == nil {
			t.Fatal("expected trust_score in posture response")
		}
	})

	t.Run("report posture with missing device_id", func(t *testing.T) {
		body := map[string]any{
			"os_type": "linux",
		}
		resp := authRequest(t, "POST", "/api/v1/ztna/posture", body, adminJWT)
		expectStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("get trust history after posture report", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/ztna/trust-history/"+deviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	// Clean up.
	delDev := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
	expectStatus(t, delDev, http.StatusNoContent)
	delDev.Body.Close()
}

func TestZTNADNSRules(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var netID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)
	if err != nil {
		t.Fatalf("failed to get default network: %v", err)
	}

	var ruleID string

	t.Run("create DNS rule", func(t *testing.T) {
		body := map[string]any{
			"network_id": netID,
			"domain":     "internal.corp.local",
			"dns_server": "10.0.0.53",
		}
		resp := authRequest(t, "POST", "/api/v1/ztna/dns-rules", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var rule map[string]any
		decodeJSON(t, resp, &rule)
		id, ok := rule["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected DNS rule id in response")
		}
		ruleID = id
	})

	t.Run("list DNS rules", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/ztna/dns-rules", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var rules []map[string]any
		decodeJSON(t, resp, &rules)
		if len(rules) == 0 {
			t.Fatal("expected at least one DNS rule")
		}
	})

	t.Run("delete DNS rule", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/ztna/dns-rules/"+ruleID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestProxyServerCRUD(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var proxyID string

	t.Run("create proxy server", func(t *testing.T) {
		body := map[string]any{
			"name":    "e2e-http-proxy",
			"type":    "http",
			"address": "proxy.e2e.test",
			"port":    8080,
		}
		resp := authRequest(t, "POST", "/api/v1/smart-routes/proxy-servers", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var ps map[string]any
		decodeJSON(t, resp, &ps)
		id, ok := ps["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected proxy server id in response")
		}
		proxyID = id
	})

	t.Run("get proxy server", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/smart-routes/proxy-servers/"+proxyID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var ps map[string]any
		decodeJSON(t, resp, &ps)
		if ps["id"] != proxyID {
			t.Fatalf("expected proxy server id %s, got %v", proxyID, ps["id"])
		}
	})

	t.Run("update proxy server", func(t *testing.T) {
		body := map[string]any{
			"name": "e2e-http-proxy-updated",
			"port": 8888,
		}
		resp := authRequest(t, "PUT", "/api/v1/smart-routes/proxy-servers/"+proxyID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var ps map[string]any
		decodeJSON(t, resp, &ps)
		if ps["name"] != "e2e-http-proxy-updated" {
			t.Fatalf("expected updated name, got %v", ps["name"])
		}
	})

	t.Run("get nonexistent proxy server returns 404", func(t *testing.T) {
		resp := authRequest(t, "GET",
			"/api/v1/smart-routes/proxy-servers/00000000-0000-0000-0000-000000000000", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("delete proxy server", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/smart-routes/proxy-servers/"+proxyID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})
}

func TestSmartRouteNetworkAssociation(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var netID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE name = 'default'`).Scan(&netID)
	if err != nil {
		t.Fatalf("failed to get default network: %v", err)
	}

	// Create a smart route to associate with a network.
	routeBody := map[string]any{
		"name":        "e2e-net-assoc-route",
		"description": "Test network association",
	}
	routeResp := authRequest(t, "POST", "/api/v1/smart-routes", routeBody, adminJWT)
	expectStatus(t, routeResp, http.StatusCreated)
	var route map[string]any
	decodeJSON(t, routeResp, &route)
	routeID := route["id"].(string)

	t.Run("associate network", func(t *testing.T) {
		body := map[string]any{
			"network_id": netID,
		}
		resp := authRequest(t, "POST", "/api/v1/smart-routes/"+routeID+"/networks", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	})

	t.Run("list route networks", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/smart-routes/"+routeID+"/networks", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var networks []map[string]any
		decodeJSON(t, resp, &networks)
		if len(networks) == 0 {
			t.Fatal("expected at least one associated network")
		}
	})

	t.Run("remove network association", func(t *testing.T) {
		resp := authRequest(t, "DELETE",
			"/api/v1/smart-routes/"+routeID+"/networks/"+netID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	})

	// Clean up.
	delRoute := authRequest(t, "DELETE", "/api/v1/smart-routes/"+routeID, nil, adminJWT)
	expectStatus(t, delRoute, http.StatusNoContent)
	delRoute.Body.Close()
}

func TestDeviceMyDevices(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("list my devices", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/my", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var devices []map[string]any
		decodeJSON(t, resp, &devices)
		// May be empty; just verify the endpoint works.
	})
}

func TestAuthForgotResetPassword(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	t.Run("forgot password with unknown email", func(t *testing.T) {
		body := map[string]any{
			"email": "nobody@outpost.local",
		}
		resp := authRequest(t, "POST", "/api/v1/auth/forgot-password", body, "")
		// Should return 200 even for unknown emails (to prevent user enumeration)
		// or possibly 422 if mail is not configured. Either way, not 404.
		if resp.StatusCode == http.StatusNotFound {
			t.Fatal("expected forgot-password endpoint to exist, got 404")
		}
		resp.Body.Close()
	})

	t.Run("reset password with invalid token", func(t *testing.T) {
		body := map[string]any{
			"token":        "invalid-reset-token",
			"new_password": "NewP@ssword123",
		}
		resp := authRequest(t, "POST", "/api/v1/auth/reset-password", body, "")
		// Expect 400 or 401 for invalid token, not 404.
		if resp.StatusCode == http.StatusNotFound {
			t.Fatal("expected reset-password endpoint to exist, got 404")
		}
		resp.Body.Close()
	})
}

func TestDeviceSendConfig(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create a device for testing send-config.
	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	devBody := map[string]any{
		"name":             "send-config-test",
		"wireguard_pubkey": "auto-generated",
		"user_id":          adminUserID,
	}
	devResp := authRequest(t, "POST", "/api/v1/devices", devBody, adminJWT)
	expectStatus(t, devResp, http.StatusCreated)
	var dev map[string]any
	decodeJSON(t, devResp, &dev)
	deviceID := dev["id"].(string)

	t.Run("send config without mail configured", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/send-config", nil, adminJWT)
		// Without SMTP, expect 422 (unprocessable) — but not 404.
		if resp.StatusCode == http.StatusNotFound {
			t.Fatal("expected send-config endpoint to exist, got 404")
		}
		resp.Body.Close()
	})

	// Clean up.
	delDev := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
	expectStatus(t, delDev, http.StatusNoContent)
	delDev.Body.Close()
}
