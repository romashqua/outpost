package wireguard

import (
	"encoding/base64"
	"testing"
)

func TestGeneratePrivateKey(t *testing.T) {
	key, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error: %v", err)
	}

	// WireGuard keys are 32 bytes, base64 encoded (44 chars with padding).
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("key is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(decoded))
	}
}

func TestGeneratePrivateKey_Unique(t *testing.T) {
	key1, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("first GeneratePrivateKey() error: %v", err)
	}
	key2, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("second GeneratePrivateKey() error: %v", err)
	}
	if key1 == key2 {
		t.Error("two generated private keys should not be equal")
	}
}

func TestPublicKey_DeriveFromPrivate(t *testing.T) {
	privKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error: %v", err)
	}

	pubKey, err := PublicKey(privKey)
	if err != nil {
		t.Fatalf("PublicKey() error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(pubKey)
	if err != nil {
		t.Fatalf("public key is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("expected 32 bytes for public key, got %d", len(decoded))
	}

	// Public key must differ from private key.
	if pubKey == privKey {
		t.Error("public key should not equal private key")
	}
}

func TestPublicKey_Deterministic(t *testing.T) {
	privKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey() error: %v", err)
	}

	pub1, err := PublicKey(privKey)
	if err != nil {
		t.Fatalf("first PublicKey() error: %v", err)
	}
	pub2, err := PublicKey(privKey)
	if err != nil {
		t.Fatalf("second PublicKey() error: %v", err)
	}

	if pub1 != pub2 {
		t.Error("deriving public key from the same private key should be deterministic")
	}
}

func TestPublicKey_InvalidInput(t *testing.T) {
	_, err := PublicKey("not-a-valid-key")
	if err == nil {
		t.Error("PublicKey() with invalid input should return error")
	}
}

func TestGeneratePresharedKey(t *testing.T) {
	key, err := GeneratePresharedKey()
	if err != nil {
		t.Fatalf("GeneratePresharedKey() error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("preshared key is not valid base64: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("expected 32 bytes for preshared key, got %d", len(decoded))
	}
}

func TestGeneratePresharedKey_Unique(t *testing.T) {
	key1, err := GeneratePresharedKey()
	if err != nil {
		t.Fatalf("first GeneratePresharedKey() error: %v", err)
	}
	key2, err := GeneratePresharedKey()
	if err != nil {
		t.Fatalf("second GeneratePresharedKey() error: %v", err)
	}
	if key1 == key2 {
		t.Error("two generated preshared keys should not be equal")
	}
}
