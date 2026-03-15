package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"testing"
)

// loginAs creates a user (via admin), logs in as that user, and returns
// the JWT token and user ID. The user is cleaned up via t.Cleanup.
func loginAs(t *testing.T, username, password string, isAdmin bool) (token, userID string) {
	t.Helper()

	body := map[string]any{
		"username":   username,
		"email":      username + "@outpost.local",
		"password":   password,
		"first_name": "Test",
		"last_name":  "User",
		"is_admin":   isAdmin,
	}
	resp := authRequest(t, "POST", "/api/v1/users", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)

	var user map[string]any
	decodeJSON(t, resp, &user)
	userID = user["id"].(string)

	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/users/"+userID, nil, adminJWT)
		resp.Body.Close()
	})

	loginBody := map[string]string{
		"username": username,
		"password": password,
	}
	loginResp := authRequest(t, "POST", "/api/v1/auth/login", loginBody, "")
	expectStatus(t, loginResp, http.StatusOK)

	var loginResult struct {
		Token string `json:"token"`
	}
	decodeJSON(t, loginResp, &loginResult)
	if loginResult.Token == "" {
		t.Fatalf("failed to get JWT for user %s", username)
	}
	return loginResult.Token, userID
}

// generateValidWireGuardKey creates a random valid 32-byte base64 WireGuard key.
func generateValidWireGuardKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

// TestDeviceSecurity_AdminOnlyList verifies that GET /devices returns 403 for
// non-admin users.
func TestDeviceSecurity_AdminOnlyList(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	userToken, _ := loginAs(t, "seclist-user", "SecureP@ss1", false)

	t.Run("non-admin gets 403 on device list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices", nil, userToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("admin gets 200 on device list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

// TestDeviceSecurity_IDOR verifies that non-admin users cannot access, download
// config for, or delete devices belonging to other users.
func TestDeviceSecurity_IDOR(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Create two non-admin users.
	userAToken, userAID := loginAs(t, "idor-usera", "SecureP@ss1", false)
	userBToken, _ := loginAs(t, "idor-userb", "SecureP@ss2", false)

	// Create a device owned by user A (as admin, since create requires user_id).
	devBody := map[string]any{
		"name":             "idor-device-a",
		"wireguard_pubkey": "auto-generated",
		"user_id":          userAID,
	}
	devResp := authRequest(t, "POST", "/api/v1/devices", devBody, adminJWT)
	expectStatus(t, devResp, http.StatusCreated)

	var dev map[string]any
	decodeJSON(t, devResp, &dev)
	deviceID := dev["id"].(string)

	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		resp.Body.Close()
	})

	t.Run("owner can view own device", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, userAToken)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("other user gets 403 on GET device", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, userBToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("other user gets 403 on GET device config", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID+"/config", nil, userBToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("other user gets 403 on DELETE device", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, userBToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("admin can view any device", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("admin can delete any device", func(t *testing.T) {
		// Create another device to test admin delete.
		body2 := map[string]any{
			"name":             "idor-device-a2",
			"wireguard_pubkey": "auto-generated",
			"user_id":          userAID,
		}
		resp2 := authRequest(t, "POST", "/api/v1/devices", body2, adminJWT)
		expectStatus(t, resp2, http.StatusCreated)
		var dev2 map[string]any
		decodeJSON(t, resp2, &dev2)
		dev2ID := dev2["id"].(string)

		delResp := authRequest(t, "DELETE", "/api/v1/devices/"+dev2ID, nil, adminJWT)
		expectStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()
	})
}

// TestDeviceSecurity_PubkeyValidation verifies that creating and enrolling
// devices validates the WireGuard public key format.
func TestDeviceSecurity_PubkeyValidation(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	t.Run("create device with invalid pubkey returns 400", func(t *testing.T) {
		body := map[string]any{
			"name":             "bad-key-device",
			"wireguard_pubkey": "not-a-valid-base64-key",
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
		expectStatus(t, resp, http.StatusBadRequest)

		var errResp map[string]any
		decodeJSON(t, resp, &errResp)
		msg, _ := errResp["message"].(string)
		if msg == "" {
			t.Fatal("expected error message in response")
		}
	})

	t.Run("create device with short key returns 400", func(t *testing.T) {
		body := map[string]any{
			"name":             "short-key-device",
			"wireguard_pubkey": "dGVzdA==", // valid base64 but only 4 bytes
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
		expectStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("create device with valid pubkey succeeds", func(t *testing.T) {
		pubkey := generateValidWireGuardKey(t)
		body := map[string]any{
			"name":             "good-key-device",
			"wireguard_pubkey": pubkey,
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var dev map[string]any
		decodeJSON(t, resp, &dev)
		devID := dev["id"].(string)

		// Clean up.
		delResp := authRequest(t, "DELETE", "/api/v1/devices/"+devID, nil, adminJWT)
		expectStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()
	})

	t.Run("enroll with invalid pubkey returns 400", func(t *testing.T) {
		body := map[string]any{
			"name":             "bad-enroll-device",
			"wireguard_pubkey": "ZZZZZZZZZZZZZZZZZZZZZZZZ",
		}
		resp := authRequest(t, "POST", "/api/v1/devices/enroll", body, adminJWT)
		expectStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})

	t.Run("enroll with empty pubkey returns 400", func(t *testing.T) {
		body := map[string]any{
			"name":             "no-key-enroll",
			"wireguard_pubkey": "",
		}
		resp := authRequest(t, "POST", "/api/v1/devices/enroll", body, adminJWT)
		expectStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	})
}

// TestDeviceSecurity_SendConfigNoMailer verifies that POST /devices/{id}/send-config
// returns 422 when no mailer is configured (which is the case in tests).
func TestDeviceSecurity_SendConfigNoMailer(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	// Create a device to test against.
	body := map[string]any{
		"name":             "sendcfg-test-device",
		"wireguard_pubkey": "auto-generated",
		"user_id":          adminUserID,
	}
	resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)

	var dev map[string]any
	decodeJSON(t, resp, &dev)
	deviceID := dev["id"].(string)

	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		resp.Body.Close()
	})

	t.Run("send-config returns 422 without mailer", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/send-config", nil, adminJWT)
		expectStatus(t, resp, http.StatusUnprocessableEntity)

		var errResp map[string]any
		decodeJSON(t, resp, &errResp)
		msg, _ := errResp["message"].(string)
		if msg == "" {
			t.Fatal("expected error message in response")
		}
	})
}

// TestDeviceSecurity_ApproveRevokeAdminOnly verifies that approve and revoke
// endpoints require admin role.
func TestDeviceSecurity_ApproveRevokeAdminOnly(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	userToken, userID := loginAs(t, "approvetest-user", "SecureP@ss1", false)

	// Create a device for this user.
	body := map[string]any{
		"name":             "approve-test-device",
		"wireguard_pubkey": "auto-generated",
		"user_id":          userID,
	}
	resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)

	var dev map[string]any
	decodeJSON(t, resp, &dev)
	deviceID := dev["id"].(string)

	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		resp.Body.Close()
	})

	t.Run("non-admin cannot approve device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/approve", nil, userToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("non-admin cannot revoke device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/revoke", nil, userToken)
		expectStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	})

	t.Run("admin can approve device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/approve", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("admin can revoke device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/revoke", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})
}

// TestDeviceSecurity_ListMy verifies that GET /devices/my returns only the
// authenticated user's devices.
func TestDeviceSecurity_ListMy(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	userToken, userID := loginAs(t, "listmy-user", "SecureP@ss1", false)

	// Create a device for the user.
	body := map[string]any{
		"name":             "my-device-1",
		"wireguard_pubkey": "auto-generated",
		"user_id":          userID,
	}
	resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)
	var dev map[string]any
	decodeJSON(t, resp, &dev)
	deviceID := dev["id"].(string)

	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		resp.Body.Close()
	})

	t.Run("list my devices returns only own devices", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/my", nil, userToken)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Devices []map[string]any `json:"devices"`
			Total   int              `json:"total"`
			Page    int              `json:"page"`
			PerPage int              `json:"per_page"`
		}
		decodeJSON(t, resp, &result)

		if result.Total < 1 {
			t.Fatalf("expected at least 1 device for user, got total=%d", result.Total)
		}
		for _, d := range result.Devices {
			if d["user_id"] != userID {
				t.Fatalf("expected all devices to belong to user %s, got user_id=%v", userID, d["user_id"])
			}
		}
	})
}
