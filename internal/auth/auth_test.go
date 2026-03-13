package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- Password tests ---

func TestHashPassword_RoundTrip(t *testing.T) {
	password := "s3cureP@ssw0rd!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	if hash == password {
		t.Error("hash should not equal plaintext password")
	}

	if err := CheckPassword(hash, password); err != nil {
		t.Errorf("CheckPassword() should succeed for correct password: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-password")
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}

	if err := CheckPassword(hash, "wrong-password"); err == nil {
		t.Error("CheckPassword() should fail for wrong password")
	}
}

func TestHashPassword_DifferentHashesForSamePassword(t *testing.T) {
	password := "same-password"
	hash1, err := HashPassword(password)
	if err != nil {
		t.Fatalf("first HashPassword() error: %v", err)
	}
	hash2, err := HashPassword(password)
	if err != nil {
		t.Fatalf("second HashPassword() error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("bcrypt should produce different hashes for the same password (different salts)")
	}

	// Both should still verify.
	if err := CheckPassword(hash1, password); err != nil {
		t.Errorf("first hash should verify: %v", err)
	}
	if err := CheckPassword(hash2, password); err != nil {
		t.Errorf("second hash should verify: %v", err)
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword() error with empty password: %v", err)
	}

	if err := CheckPassword(hash, ""); err != nil {
		t.Errorf("CheckPassword() should succeed for empty password: %v", err)
	}

	if err := CheckPassword(hash, "non-empty"); err == nil {
		t.Error("CheckPassword() should fail when comparing empty hash with non-empty password")
	}
}

// --- JWT tests ---

func TestGenerateToken_RoundTrip(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	claims := TokenClaims{
		UserID:   "user-123",
		Username: "testuser",
		Email:    "test@example.com",
		IsAdmin:  true,
		Roles:    []string{"admin", "user"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	tokenStr, err := GenerateToken(secret, claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("GenerateToken() returned empty token")
	}

	got, err := ValidateToken(secret, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken() error: %v", err)
	}

	if got.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", got.UserID, "user-123")
	}
	if got.Username != "testuser" {
		t.Errorf("Username = %q, want %q", got.Username, "testuser")
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "test@example.com")
	}
	if !got.IsAdmin {
		t.Error("IsAdmin should be true")
	}
	if len(got.Roles) != 2 || got.Roles[0] != "admin" || got.Roles[1] != "user" {
		t.Errorf("Roles = %v, want [admin user]", got.Roles)
	}
}

func TestGenerateToken_DefaultExpiry(t *testing.T) {
	secret := "test-secret"
	claims := TokenClaims{
		UserID:   "user-456",
		Username: "defaultexpiry",
	}

	tokenStr, err := GenerateToken(secret, claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	got, err := ValidateToken(secret, tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken() error: %v", err)
	}

	if got.ExpiresAt == nil {
		t.Fatal("expected default ExpiresAt to be set")
	}

	// Default expiry should be roughly 24 hours from now.
	expectedExpiry := time.Now().Add(24 * time.Hour)
	diff := got.ExpiresAt.Time.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("default expiry should be ~24h from now, got %v", got.ExpiresAt.Time)
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	secret := "test-secret"
	claims := TokenClaims{
		UserID:   "user-expired",
		Username: "expireduser",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}

	tokenStr, err := GenerateToken(secret, claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	_, err = ValidateToken(secret, tokenStr)
	if err == nil {
		t.Error("ValidateToken() should fail for expired token")
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	claims := TokenClaims{
		UserID:   "user-789",
		Username: "wrongsig",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}

	tokenStr, err := GenerateToken("correct-secret", claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	_, err = ValidateToken("wrong-secret", tokenStr)
	if err == nil {
		t.Error("ValidateToken() should fail with wrong secret")
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	_, err := ValidateToken("secret", "not.a.valid.jwt.token")
	if err == nil {
		t.Error("ValidateToken() should fail for malformed token")
	}
}

func TestValidateToken_EmptyToken(t *testing.T) {
	_, err := ValidateToken("secret", "")
	if err == nil {
		t.Error("ValidateToken() should fail for empty token string")
	}
}
