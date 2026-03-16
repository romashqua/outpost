package pki

import (
	"encoding/json"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestDefaultKeyLifetime(t *testing.T) {
	expected := 90 * 24 * time.Hour
	if DefaultKeyLifetime != expected {
		t.Errorf("DefaultKeyLifetime=%v, want %v", DefaultKeyLifetime, expected)
	}
	// Sanity: should be exactly 90 days.
	if DefaultKeyLifetime.Hours() != 2160 {
		t.Errorf("DefaultKeyLifetime hours=%f, want 2160", DefaultKeyLifetime.Hours())
	}
}

func TestKeyPair_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	kp := KeyPair{
		ID:         "kp-1",
		DeviceID:   "dev-42",
		PublicKey:  "base64pubkey==",
		CreatedAt:  now,
		ExpiresAt:  now.Add(DefaultKeyLifetime),
		IsActive:   true,
		RotationID: 3,
	}

	data, err := json.Marshal(kp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded KeyPair
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != "kp-1" {
		t.Errorf("ID=%q, want kp-1", decoded.ID)
	}
	if decoded.DeviceID != "dev-42" {
		t.Errorf("DeviceID=%q, want dev-42", decoded.DeviceID)
	}
	if decoded.PublicKey != "base64pubkey==" {
		t.Errorf("PublicKey=%q, want base64pubkey==", decoded.PublicKey)
	}
	if decoded.RotationID != 3 {
		t.Errorf("RotationID=%d, want 3", decoded.RotationID)
	}
	if decoded.IsActive != true {
		t.Error("IsActive should be true")
	}
	if !decoded.ExpiresAt.After(decoded.CreatedAt) {
		t.Error("ExpiresAt should be after CreatedAt")
	}
}

func TestKeyPair_JSONKeys(t *testing.T) {
	kp := KeyPair{
		ID:         "x",
		DeviceID:   "d",
		GatewayID:  "g",
		PublicKey:  "pk",
		IsActive:   true,
		RotationID: 1,
	}

	data, err := json.Marshal(kp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	expectedKeys := []string{"id", "public_key", "created_at", "expires_at", "is_active", "rotation_id"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestKeyPair_OmitEmptyDeviceGateway(t *testing.T) {
	kp := KeyPair{
		ID:        "x",
		PublicKey: "pk",
	}

	data, err := json.Marshal(kp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// device_id and gateway_id should be omitted when empty due to omitempty.
	if _, ok := raw["device_id"]; ok {
		t.Error("device_id should be omitted when empty")
	}
	if _, ok := raw["gateway_id"]; ok {
		t.Error("gateway_id should be omitted when empty")
	}
}

func TestWireGuardKeyGeneration(t *testing.T) {
	// Test that the WireGuard key generation library works as expected.
	// This is the same function used by RotateDeviceKey and RotateGatewayKey.
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey error: %v", err)
	}

	pubKey := key.PublicKey()

	// Private key should be 32 bytes (base64-encoded to 44 chars).
	privStr := key.String()
	if len(privStr) != 44 {
		t.Errorf("private key string length=%d, want 44", len(privStr))
	}

	// Public key should also be 44 chars base64.
	pubStr := pubKey.String()
	if len(pubStr) != 44 {
		t.Errorf("public key string length=%d, want 44", len(pubStr))
	}

	// Keys should not be equal.
	if privStr == pubStr {
		t.Error("private and public key strings should differ")
	}
}

func TestWireGuardKeyUniqueness(t *testing.T) {
	// Generate multiple keys and confirm they are all different.
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		key, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			t.Fatalf("GeneratePrivateKey error on iteration %d: %v", i, err)
		}
		pubStr := key.PublicKey().String()
		if seen[pubStr] {
			t.Fatalf("duplicate public key generated: %s", pubStr)
		}
		seen[pubStr] = true
	}
}

func TestWireGuardKeyDeterministicPublic(t *testing.T) {
	// The same private key should always produce the same public key.
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey error: %v", err)
	}

	pub1 := key.PublicKey().String()
	pub2 := key.PublicKey().String()

	if pub1 != pub2 {
		t.Errorf("same private key produced different public keys: %q vs %q", pub1, pub2)
	}
}

func TestKeyPairExpiration(t *testing.T) {
	now := time.Now().UTC()
	kp := KeyPair{
		CreatedAt: now,
		ExpiresAt: now.Add(DefaultKeyLifetime),
	}

	// Should not be expired right now.
	if time.Now().After(kp.ExpiresAt) {
		t.Error("key pair should not be expired immediately after creation")
	}

	// Simulate an expired key.
	kp.ExpiresAt = now.Add(-time.Hour)
	if !time.Now().After(kp.ExpiresAt) {
		t.Error("key pair with past ExpiresAt should be considered expired")
	}
}

func TestNewManager(t *testing.T) {
	// NewManager with nil pool should not panic (it just stores the reference).
	m := NewManager(nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.pool != nil {
		t.Error("pool should be nil when passed nil")
	}
}
