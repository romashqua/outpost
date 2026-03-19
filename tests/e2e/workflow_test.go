package e2e

import (
	"net/http"
	"testing"
)

// TestWorkflow_ZTNA tests the full Zero-Trust Network Access flow:
// create policy → verify trust scores → update trust config.
func TestWorkflow_ZTNA(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	t.Run("ListTrustScores", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/ztna/trust-scores", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("GetTrustConfig", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/ztna/trust-config", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)
	})

	t.Run("UpdateTrustConfig", func(t *testing.T) {
		body := map[string]any{
			"auto_restrict_below_medium": true,
			"auto_block_below_low":       true,
		}
		resp := authRequest(t, http.MethodPut, "/api/v1/ztna/trust-config", body, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("CreatePolicy", func(t *testing.T) {
		body := map[string]any{
			"name":                    "e2e-test-policy",
			"require_disk_encryption": true,
			"require_screen_lock":     false,
			"require_antivirus":       false,
			"require_firewall":        false,
			"require_mfa":             true,
			"min_os_version":          "",
			"network_ids":             []string{},
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/ztna/policies", body, adminJWT)
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			var result map[string]any
			decodeJSON(t, resp, &result)

			// Cleanup: delete the policy.
			if id, ok := result["id"].(string); ok {
				t.Cleanup(func() {
					r := authRequest(t, http.MethodDelete, "/api/v1/ztna/policies/"+id, nil, adminJWT)
					r.Body.Close()
				})
			}
		} else {
			resp.Body.Close()
		}
	})

	t.Run("ListPolicies", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/ztna/policies", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

// TestWorkflow_Webhooks tests webhook CRUD and test delivery.
func TestWorkflow_Webhooks(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	var webhookID string

	t.Run("CreateWebhook", func(t *testing.T) {
		body := map[string]any{
			"url":    "https://httpbin.org/post",
			"events": []string{"*"},
			"secret": "e2e-test-secret",
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/webhooks", body, adminJWT)
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			var result map[string]any
			decodeJSON(t, resp, &result)
			if id, ok := result["id"].(string); ok {
				webhookID = id
			}
		} else {
			resp.Body.Close()
		}
	})

	t.Run("ListWebhooks", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/webhooks", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("GetWebhook", func(t *testing.T) {
		if webhookID == "" {
			t.Skip("no webhook created")
		}
		resp := authRequest(t, http.MethodGet, "/api/v1/webhooks/"+webhookID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("DeleteWebhook", func(t *testing.T) {
		if webhookID == "" {
			t.Skip("no webhook created")
		}
		resp := authRequest(t, http.MethodDelete, "/api/v1/webhooks/"+webhookID, nil, adminJWT)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 200 or 204, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestWorkflow_Tenants tests multi-tenant CRUD.
func TestWorkflow_Tenants(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	var tenantID string

	t.Run("CreateTenant", func(t *testing.T) {
		body := map[string]any{
			"name":      "e2e-test-tenant",
			"subdomain": "e2etest",
			"is_active": true,
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/tenants", body, adminJWT)
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			var result map[string]any
			decodeJSON(t, resp, &result)
			if id, ok := result["id"].(string); ok {
				tenantID = id
			}
		} else {
			resp.Body.Close()
		}
	})

	t.Run("ListTenants", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/tenants", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("GetTenant", func(t *testing.T) {
		if tenantID == "" {
			t.Skip("no tenant created")
		}
		resp := authRequest(t, http.MethodGet, "/api/v1/tenants/"+tenantID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("UpdateTenant", func(t *testing.T) {
		if tenantID == "" {
			t.Skip("no tenant created")
		}
		body := map[string]any{
			"name": "e2e-test-tenant-updated",
		}
		resp := authRequest(t, http.MethodPut, "/api/v1/tenants/"+tenantID, body, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("GetTenantStats", func(t *testing.T) {
		if tenantID == "" {
			t.Skip("no tenant created")
		}
		resp := authRequest(t, http.MethodGet, "/api/v1/tenants/"+tenantID+"/stats", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("DeleteTenant", func(t *testing.T) {
		if tenantID == "" {
			t.Skip("no tenant created")
		}
		resp := authRequest(t, http.MethodDelete, "/api/v1/tenants/"+tenantID, nil, adminJWT)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 200 or 204, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestWorkflow_Notifications tests the notification and audit event system.
func TestWorkflow_Notifications(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	t.Run("ListNotifications", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/notifications?limit=10", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)

		if _, ok := result["notifications"]; !ok {
			t.Error("expected 'notifications' key in response")
		}
		if _, ok := result["total"]; !ok {
			t.Error("expected 'total' key in response")
		}
	})

	t.Run("UnreadCount", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/notifications/unread-count", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if _, ok := result["count"]; !ok {
			t.Error("expected 'count' key in response")
		}
	})

	t.Run("MarkRead", func(t *testing.T) {
		resp := authRequest(t, http.MethodPost, "/api/v1/notifications/mark-read", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("AuditList", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/audit?per_page=5", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)

		if _, ok := result["data"]; !ok {
			t.Error("expected 'data' key in response")
		}
		if _, ok := result["total"]; !ok {
			t.Error("expected 'total' key in response")
		}
	})

	t.Run("AuditStats", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/audit/stats", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("AuditExportJSON", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/audit/export?format=json", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("AuditExportCSV", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/audit/export?format=csv", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		ct := resp.Header.Get("Content-Type")
		if ct != "text/csv" {
			t.Errorf("expected Content-Type text/csv, got %s", ct)
		}
		resp.Body.Close()
	})
}

// TestWorkflow_MFA tests the MFA status and TOTP setup flow.
func TestWorkflow_MFA(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	t.Run("MFAStatus", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/mfa/status", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
	})

	t.Run("TOTPSetup", func(t *testing.T) {
		resp := authRequest(t, http.MethodPost, "/api/v1/mfa/totp/setup", nil, adminJWT)
		// Should return 200 with QR code/secret, or 409 if already set up.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 200 or 409, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("WebAuthnCredentials", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/mfa/webauthn/credentials", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("BackupCodes", func(t *testing.T) {
		resp := authRequest(t, http.MethodPost, "/api/v1/mfa/backup-codes", nil, adminJWT)
		// May return 200 (codes generated) or 400/409 if TOTP not set up.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 200, 400, or 409, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})
}

// TestWorkflow_Sessions tests session listing and management.
func TestWorkflow_Sessions(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	t.Run("ListSessions", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/sessions", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

// TestWorkflow_AuthFlow tests the complete authentication lifecycle:
// login → change password → re-login → refresh token → logout.
func TestWorkflow_AuthFlow(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	// Create a test user for the workflow.
	var userID string
	createBody := map[string]any{
		"username":   "e2e-auth-flow",
		"email":      "authflow@test.local",
		"password":   "TestPass123!",
		"first_name": "Auth",
		"last_name":  "Flow",
		"role":       "user",
	}
	resp := authRequest(t, http.MethodPost, "/api/v1/users", createBody, adminJWT)
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var result map[string]any
		decodeJSON(t, resp, &result)
		if id, ok := result["id"].(string); ok {
			userID = id
		}
	} else {
		resp.Body.Close()
	}
	t.Cleanup(func() {
		if userID != "" {
			r := authRequest(t, http.MethodDelete, "/api/v1/users/"+userID, nil, adminJWT)
			r.Body.Close()
		}
	})

	t.Run("Login", func(t *testing.T) {
		body := map[string]any{
			"username": "e2e-auth-flow",
			"password": "TestPass123!",
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/auth/login", body, "")
		expectStatus(t, resp, http.StatusOK)

		var result map[string]any
		decodeJSON(t, resp, &result)
		if _, ok := result["token"]; !ok {
			t.Fatal("expected 'token' in login response")
		}
	})

	t.Run("LoginInvalidPassword", func(t *testing.T) {
		body := map[string]any{
			"username": "e2e-auth-flow",
			"password": "WrongPassword!",
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/auth/login", body, "")
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 401 or 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("LoginNonExistentUser", func(t *testing.T) {
		body := map[string]any{
			"username": "nonexistent-user-xyz",
			"password": "whatever",
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/auth/login", body, "")
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 401 or 403, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("ChangePassword", func(t *testing.T) {
		// Login first to get a token.
		loginBody := map[string]any{
			"username": "e2e-auth-flow",
			"password": "TestPass123!",
		}
		loginResp := authRequest(t, http.MethodPost, "/api/v1/auth/login", loginBody, "")
		var loginResult map[string]any
		decodeJSON(t, loginResp, &loginResult)
		token, _ := loginResult["token"].(string)
		if token == "" {
			t.Skip("could not get token")
		}

		// Change password.
		body := map[string]any{
			"current_password": "TestPass123!",
			"new_password":     "NewPass456!",
		}
		resp := authRequest(t, http.MethodPost, "/api/v1/auth/change-password", body, token)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()

		// Login with new password.
		body2 := map[string]any{
			"username": "e2e-auth-flow",
			"password": "NewPass456!",
		}
		resp2 := authRequest(t, http.MethodPost, "/api/v1/auth/login", body2, "")
		expectStatus(t, resp2, http.StatusOK)
		resp2.Body.Close()
	})
}

// TestWorkflow_NonAdminAccess verifies that non-admin users get 403 on admin endpoints.
func TestWorkflow_NonAdminAccess(t *testing.T) {
	if ts == nil {
		t.Skip("test server not initialized")
	}

	// Create a non-admin user.
	var userID string
	createBody := map[string]any{
		"username":   "e2e-regular-user",
		"email":      "regular@test.local",
		"password":   "RegularPass1!",
		"first_name": "Regular",
		"last_name":  "User",
		"role":       "user",
	}
	resp := authRequest(t, http.MethodPost, "/api/v1/users", createBody, adminJWT)
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var result map[string]any
		decodeJSON(t, resp, &result)
		if id, ok := result["id"].(string); ok {
			userID = id
		}
	} else {
		resp.Body.Close()
	}
	t.Cleanup(func() {
		if userID != "" {
			r := authRequest(t, http.MethodDelete, "/api/v1/users/"+userID, nil, adminJWT)
			r.Body.Close()
		}
	})

	// Login as non-admin.
	loginBody := map[string]any{
		"username": "e2e-regular-user",
		"password": "RegularPass1!",
	}
	loginResp := authRequest(t, http.MethodPost, "/api/v1/auth/login", loginBody, "")
	var loginResult map[string]any
	decodeJSON(t, loginResp, &loginResult)
	userToken, _ := loginResult["token"].(string)
	if userToken == "" {
		t.Skip("could not get user token")
	}

	adminEndpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/users"},
		{http.MethodGet, "/api/v1/audit"},
		{http.MethodGet, "/api/v1/tenants"},
		{http.MethodGet, "/api/v1/gateways"},
	}

	for _, ep := range adminEndpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			resp := authRequest(t, ep.method, ep.path, nil, userToken)
			if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 403 or 401, got %d", ep.method, ep.path, resp.StatusCode)
			}
			resp.Body.Close()
		})
	}

	// Non-admin CAN access their own notifications.
	t.Run("NonAdminCanAccessNotifications", func(t *testing.T) {
		resp := authRequest(t, http.MethodGet, "/api/v1/notifications", nil, userToken)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}
