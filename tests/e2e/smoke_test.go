package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

// smokeBaseURL returns the live server URL from the OUTPOST_API_URL env var.
// Tests in this file are standalone HTTP contract tests that run against an
// already-running Outpost instance -- they do not require a test DB and skip
// gracefully when the env var is absent.
func smokeBaseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("OUTPOST_API_URL")
	if u == "" {
		t.Skip("OUTPOST_API_URL not set, skipping smoke tests")
	}
	return u
}

// smokeRequest builds and executes an HTTP request against the live server.
func smokeRequest(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()

	base := smokeBaseURL(t)
	url := base + path

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s %s failed: %v", method, path, err)
	}
	return resp
}

// smokeExpectStatus asserts the response status code.
func smokeExpectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected status %d, got %d; body: %s", want, resp.StatusCode, string(b))
	}
}

// smokeLogin authenticates against the live server and returns a JWT token.
func smokeLogin(t *testing.T, username, password string) string {
	t.Helper()
	body := map[string]string{"username": username, "password": password}
	resp := smokeRequest(t, "POST", "/api/v1/auth/login", body, "")
	smokeExpectStatus(t, resp, http.StatusOK)

	var result struct {
		Token string `json:"token"`
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("failed to decode login response: %v\nbody: %s", err, string(b))
	}
	if result.Token == "" {
		t.Fatal("received empty token from login")
	}
	return result.Token
}

// ---------------------------------------------------------------------------
// Smoke Tests -- API contract verification against a live server.
// ---------------------------------------------------------------------------

func TestSmoke_Healthcheck(t *testing.T) {
	resp := smokeRequest(t, "GET", "/healthz", nil, "")
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = smokeRequest(t, "GET", "/readyz", nil, "")
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_AuthFlow(t *testing.T) {
	t.Run("valid login", func(t *testing.T) {
		token := smokeLogin(t, "admin", "admin")
		if token == "" {
			t.Fatal("expected non-empty token")
		}
	})

	t.Run("invalid login", func(t *testing.T) {
		body := map[string]string{"username": "admin", "password": "wrong"}
		resp := smokeRequest(t, "POST", "/api/v1/auth/login", body, "")
		smokeExpectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})

	t.Run("unauthenticated access", func(t *testing.T) {
		resp := smokeRequest(t, "GET", "/api/v1/users", nil, "")
		smokeExpectStatus(t, resp, http.StatusUnauthorized)
		resp.Body.Close()
	})
}

func TestSmoke_Users(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	t.Run("list users", func(t *testing.T) {
		resp := smokeRequest(t, "GET", "/api/v1/users", nil, token)
		smokeExpectStatus(t, resp, http.StatusOK)

		var result map[string]any
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(b, &result); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		if result["users"] == nil {
			t.Fatal("expected 'users' key in response")
		}
	})

	t.Run("create and delete user", func(t *testing.T) {
		body := map[string]any{
			"username":   fmt.Sprintf("smoke-%d", os.Getpid()),
			"email":      fmt.Sprintf("smoke-%d@test.local", os.Getpid()),
			"password":   "SmokeTestP@ss1",
			"first_name": "Smoke",
			"last_name":  "Test",
		}
		resp := smokeRequest(t, "POST", "/api/v1/users", body, token)
		smokeExpectStatus(t, resp, http.StatusCreated)

		var user map[string]any
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(b, &user); err != nil {
			t.Fatalf("failed to decode: %v", err)
		}
		id, ok := user["id"].(string)
		if !ok || id == "" {
			t.Fatal("expected user id")
		}

		// Clean up.
		delResp := smokeRequest(t, "DELETE", "/api/v1/users/"+id, nil, token)
		smokeExpectStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()
	})
}

func TestSmoke_Networks(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/networks", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_Devices(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/devices", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_Gateways(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/gateways", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_S2STunnels(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/s2s-tunnels", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_SmartRoutes(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	t.Run("list routes", func(t *testing.T) {
		resp := smokeRequest(t, "GET", "/api/v1/smart-routes", nil, token)
		smokeExpectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("list proxy servers", func(t *testing.T) {
		resp := smokeRequest(t, "GET", "/api/v1/smart-routes/proxy-servers", nil, token)
		smokeExpectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

func TestSmoke_Settings(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/settings", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_Dashboard(t *testing.T) {
	resp := smokeRequest(t, "GET", "/api/v1/dashboard/stats", nil, "")
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_Analytics(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	for _, endpoint := range []string{
		"/api/v1/analytics/summary",
		"/api/v1/analytics/bandwidth",
		"/api/v1/analytics/top-users",
		"/api/v1/analytics/connections-heatmap",
	} {
		t.Run(endpoint, func(t *testing.T) {
			resp := smokeRequest(t, "GET", endpoint, nil, token)
			smokeExpectStatus(t, resp, http.StatusOK)
			resp.Body.Close()
		})
	}
}

func TestSmoke_Compliance(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	for _, endpoint := range []string{
		"/api/v1/compliance/report",
		"/api/v1/compliance/soc2",
		"/api/v1/compliance/iso27001",
		"/api/v1/compliance/gdpr",
	} {
		t.Run(endpoint, func(t *testing.T) {
			resp := smokeRequest(t, "GET", endpoint, nil, token)
			smokeExpectStatus(t, resp, http.StatusOK)
			resp.Body.Close()
		})
	}
}

func TestSmoke_MFA(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/mfa/status", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_Audit(t *testing.T) {
	token := smokeLogin(t, "admin", "admin")

	resp := smokeRequest(t, "GET", "/api/v1/audit", nil, token)
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestSmoke_OpenAPISpec(t *testing.T) {
	resp := smokeRequest(t, "GET", "/api/docs/openapi.yaml", nil, "")
	smokeExpectStatus(t, resp, http.StatusOK)
	ct := resp.Header.Get("Content-Type")
	if ct != "application/yaml" {
		t.Fatalf("expected Content-Type application/yaml, got %s", ct)
	}
	resp.Body.Close()
}

func TestSmoke_Metrics(t *testing.T) {
	resp := smokeRequest(t, "GET", "/metrics", nil, "")
	smokeExpectStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
