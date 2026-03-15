package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestDeviceFlow_FullLifecycle tests the complete device lifecycle:
// create -> approve -> download config -> revoke -> delete.
func TestDeviceFlow_FullLifecycle(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Get admin user ID.
	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	// Ensure we have an active network and gateway for config download.
	var netID string
	err = pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&netID)
	if err != nil {
		t.Fatalf("no active network available: %v", err)
	}

	// Create a gateway for config generation.
	gwBody := map[string]any{
		"name":       "lifecycle-test-gw",
		"network_id": netID,
		"endpoint":   "gw.lifecycle-test.local:51820",
	}
	gwResp := authRequest(t, "POST", "/api/v1/gateways", gwBody, adminJWT)
	expectStatus(t, gwResp, http.StatusCreated)
	var gw map[string]any
	decodeJSON(t, gwResp, &gw)
	gwID := gw["id"].(string)
	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
		resp.Body.Close()
	})

	var deviceID string

	t.Run("step 1: create device", func(t *testing.T) {
		body := map[string]any{
			"name":             "lifecycle-device",
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

		if dev["name"] != "lifecycle-device" {
			t.Fatalf("expected name 'lifecycle-device', got %v", dev["name"])
		}
		if dev["is_approved"] != false {
			t.Fatal("expected newly created device to be unapproved")
		}
		if dev["assigned_ip"] == nil || dev["assigned_ip"] == "" {
			t.Fatal("expected assigned_ip to be set")
		}
		if dev["wireguard_pubkey"] == nil || dev["wireguard_pubkey"] == "" {
			t.Fatal("expected wireguard_pubkey to be set (auto-generated)")
		}
	})

	t.Run("step 2: device appears in list", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result struct {
			Devices []map[string]any `json:"devices"`
			Total   int              `json:"total"`
		}
		decodeJSON(t, resp, &result)

		found := false
		for _, d := range result.Devices {
			if d["id"] == deviceID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("device %s not found in device list", deviceID)
		}
	})

	t.Run("step 3: approve device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/approve", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]string
		decodeJSON(t, resp, &result)
		if result["status"] != "approved" {
			t.Fatalf("expected status 'approved', got %q", result["status"])
		}

		// Verify the device is now approved.
		getResp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, getResp, http.StatusOK)
		var dev map[string]any
		decodeJSON(t, getResp, &dev)
		if dev["is_approved"] != true {
			t.Fatal("expected device to be approved after approve call")
		}
	})

	t.Run("step 4: download config", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+deviceID+"/config", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var cfg map[string]any
		decodeJSON(t, resp, &cfg)

		configText, _ := cfg["config"].(string)
		if configText == "" {
			t.Fatal("expected non-empty config text")
		}
		privKey, _ := cfg["private_key"].(string)
		if privKey == "" {
			t.Fatal("expected non-empty private_key")
		}
		pubKey, _ := cfg["public_key"].(string)
		if pubKey == "" {
			t.Fatal("expected non-empty public_key")
		}

		// Config should contain WireGuard sections.
		if !strings.Contains(configText, "[Interface]") {
			t.Fatal("config should contain [Interface] section")
		}
		if !strings.Contains(configText, "[Peer]") {
			t.Fatal("config should contain [Peer] section")
		}
	})

	t.Run("step 5: revoke device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+deviceID+"/revoke", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var result map[string]string
		decodeJSON(t, resp, &result)
		if result["status"] != "revoked" {
			t.Fatalf("expected status 'revoked', got %q", result["status"])
		}

		// Verify the device is now revoked.
		getResp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, getResp, http.StatusOK)
		var dev map[string]any
		decodeJSON(t, getResp, &dev)
		if dev["is_approved"] != false {
			t.Fatal("expected device to be unapproved after revoke")
		}
	})

	t.Run("step 6: delete device", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()

		// Verify it is gone.
		getResp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, getResp, http.StatusNotFound)
		getResp.Body.Close()
	})
}

// TestDeviceFlow_EnrollLifecycle tests the enrollment-based device lifecycle:
// enroll (auto-approved) -> download config -> revoke -> delete.
func TestDeviceFlow_EnrollLifecycle(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	// Ensure we have an active gateway for enrollment.
	var netID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&netID)
	if err != nil {
		t.Fatalf("no active network available: %v", err)
	}

	// Create a gateway.
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
	t.Cleanup(func() {
		resp := authRequest(t, "DELETE", "/api/v1/gateways/"+gwID, nil, adminJWT)
		resp.Body.Close()
	})

	pubkey := generateValidWireGuardKey(t)
	var enrolledDeviceID string

	t.Run("step 1: enroll device", func(t *testing.T) {
		body := map[string]any{
			"name":             "enroll-device",
			"wireguard_pubkey": pubkey,
		}
		resp := authRequest(t, "POST", "/api/v1/devices/enroll", body, adminJWT)
		expectStatus(t, resp, http.StatusCreated)

		var result struct {
			DeviceID           string   `json:"device_id"`
			Address            string   `json:"address"`
			DNS                []string `json:"dns"`
			Endpoint           string   `json:"endpoint"`
			ServerPublicKey    string   `json:"server_public_key"`
			AllowedIPs         []string `json:"allowed_ips"`
			PersistentKeepalive int     `json:"persistent_keepalive"`
		}
		decodeJSON(t, resp, &result)

		if result.DeviceID == "" {
			t.Fatal("expected device_id in enrollment response")
		}
		enrolledDeviceID = result.DeviceID

		if result.Address == "" {
			t.Fatal("expected address in enrollment response")
		}
		if result.Endpoint == "" {
			t.Fatal("expected endpoint in enrollment response")
		}
		if result.ServerPublicKey == "" {
			t.Fatal("expected server_public_key in enrollment response")
		}
		if len(result.AllowedIPs) == 0 {
			t.Fatal("expected allowed_ips in enrollment response")
		}
		if len(result.DNS) == 0 {
			t.Fatal("expected dns in enrollment response")
		}
	})

	t.Run("step 2: enrolled device is auto-approved", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+enrolledDeviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)

		var dev map[string]any
		decodeJSON(t, resp, &dev)
		if dev["is_approved"] != true {
			t.Fatal("expected enrolled device to be auto-approved")
		}
	})

	t.Run("step 3: revoke enrolled device", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+enrolledDeviceID+"/revoke", nil, adminJWT)
		expectStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	})

	t.Run("step 4: delete enrolled device", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+enrolledDeviceID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()

		// Verify it is gone.
		getResp := authRequest(t, "GET", "/api/v1/devices/"+enrolledDeviceID, nil, adminJWT)
		expectStatus(t, getResp, http.StatusNotFound)
		getResp.Body.Close()
	})
}

// TestDeviceFlow_OwnerCanDeleteOwnDevice tests that a non-admin user can
// delete their own device.
func TestDeviceFlow_OwnerCanDeleteOwnDevice(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	userToken, userID := loginAs(t, "ownerdel-user", "SecureP@ss1", false)

	// Create a device owned by this user.
	body := map[string]any{
		"name":             "owner-del-device",
		"wireguard_pubkey": "auto-generated",
		"user_id":          userID,
	}
	resp := authRequest(t, "POST", "/api/v1/devices", body, adminJWT)
	expectStatus(t, resp, http.StatusCreated)
	var dev map[string]any
	decodeJSON(t, resp, &dev)
	deviceID := dev["id"].(string)

	t.Run("owner can delete own device", func(t *testing.T) {
		delResp := authRequest(t, "DELETE", "/api/v1/devices/"+deviceID, nil, userToken)
		expectStatus(t, delResp, http.StatusNoContent)
		delResp.Body.Close()

		// Verify deleted.
		getResp := authRequest(t, "GET", "/api/v1/devices/"+deviceID, nil, adminJWT)
		expectStatus(t, getResp, http.StatusNotFound)
		getResp.Body.Close()
	})
}

// TestDeviceFlow_DuplicateDevice tests that creating a device with a duplicate
// name or public key returns 409 Conflict.
func TestDeviceFlow_DuplicateDevice(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	var adminUserID string
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE username = 'admin'`).Scan(&adminUserID)
	if err != nil {
		t.Fatalf("failed to get admin user id: %v", err)
	}

	pubkey := generateValidWireGuardKey(t)

	// Create original device.
	body := map[string]any{
		"name":             "dup-test-device",
		"wireguard_pubkey": pubkey,
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

	t.Run("duplicate pubkey returns 409", func(t *testing.T) {
		dupBody := map[string]any{
			"name":             "dup-test-device-2",
			"wireguard_pubkey": pubkey,
			"user_id":          adminUserID,
		}
		resp := authRequest(t, "POST", "/api/v1/devices", dupBody, adminJWT)
		expectStatus(t, resp, http.StatusConflict)
		resp.Body.Close()
	})
}

// TestDeviceFlow_NonexistentDevice tests operations on non-existent devices.
func TestDeviceFlow_NonexistentDevice(t *testing.T) {
	if adminJWT == "" {
		t.Skip("no admin JWT available; TestAuth must run first")
	}

	fakeID := "00000000-0000-0000-0000-000000000000"

	t.Run("get nonexistent device returns 404", func(t *testing.T) {
		resp := authRequest(t, "GET", "/api/v1/devices/"+fakeID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("delete nonexistent device returns 404", func(t *testing.T) {
		resp := authRequest(t, "DELETE", "/api/v1/devices/"+fakeID, nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("approve nonexistent device returns 404", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+fakeID+"/approve", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})

	t.Run("revoke nonexistent device returns 404", func(t *testing.T) {
		resp := authRequest(t, "POST", "/api/v1/devices/"+fakeID+"/revoke", nil, adminJWT)
		expectStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	})
}

