package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- BuildIDTokenClaims ---

func TestBuildIDTokenClaims_Basic(t *testing.T) {
	user := UserInfo{
		UserID:        "u-123",
		Email:         "alice@example.com",
		EmailVerified: true,
		Name:          "Alice",
		Username:      "alice",
		Groups:        []string{"admins", "devs"},
	}

	claims := BuildIDTokenClaims(user, "https://issuer.example.com", "client-abc", "nonce-xyz", 1*time.Hour)

	if claims.Nonce != "nonce-xyz" {
		t.Errorf("expected nonce %q, got %q", "nonce-xyz", claims.Nonce)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email %q, got %q", "alice@example.com", claims.Email)
	}
	if !claims.EmailVerified {
		t.Error("expected EmailVerified to be true")
	}
	if claims.Name != "Alice" {
		t.Errorf("expected name %q, got %q", "Alice", claims.Name)
	}
	if claims.PreferredUsername != "alice" {
		t.Errorf("expected preferred_username %q, got %q", "alice", claims.PreferredUsername)
	}
	if len(claims.Groups) != 2 || claims.Groups[0] != "admins" {
		t.Errorf("unexpected groups: %v", claims.Groups)
	}
	if claims.Issuer != "https://issuer.example.com" {
		t.Errorf("expected issuer %q, got %q", "https://issuer.example.com", claims.Issuer)
	}
	if claims.Subject != "u-123" {
		t.Errorf("expected subject %q, got %q", "u-123", claims.Subject)
	}
	aud, _ := claims.GetAudience()
	if len(aud) != 1 || aud[0] != "client-abc" {
		t.Errorf("unexpected audience: %v", aud)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt should not be nil")
	}
	// Expiry should be roughly 1 hour from now.
	diff := time.Until(claims.ExpiresAt.Time)
	if diff < 59*time.Minute || diff > 61*time.Minute {
		t.Errorf("expected expiry ~1h from now, got %v", diff)
	}
}

func TestBuildIDTokenClaims_EmptyGroups(t *testing.T) {
	user := UserInfo{
		UserID: "u-456",
		Email:  "bob@example.com",
	}
	claims := BuildIDTokenClaims(user, "https://iss", "cid", "", 30*time.Minute)
	if claims.Nonce != "" {
		t.Errorf("expected empty nonce, got %q", claims.Nonce)
	}
	if claims.Groups != nil {
		t.Errorf("expected nil groups, got %v", claims.Groups)
	}
}

// --- ValidateRedirectURI ---

func TestValidateRedirectURI_Match(t *testing.T) {
	client := &Client{
		RedirectURIs: []string{
			"https://app.example.com/callback",
			"http://localhost:8080/callback",
		},
	}
	if !ValidateRedirectURI(client, "https://app.example.com/callback") {
		t.Error("expected matching redirect URI to be valid")
	}
	if !ValidateRedirectURI(client, "http://localhost:8080/callback") {
		t.Error("expected localhost callback to be valid")
	}
}

func TestValidateRedirectURI_NoMatch(t *testing.T) {
	client := &Client{
		RedirectURIs: []string{"https://app.example.com/callback"},
	}
	if ValidateRedirectURI(client, "https://evil.com/callback") {
		t.Error("expected non-matching redirect URI to be invalid")
	}
}

func TestValidateRedirectURI_EmptyList(t *testing.T) {
	client := &Client{RedirectURIs: nil}
	if ValidateRedirectURI(client, "https://any.com") {
		t.Error("expected empty redirect URI list to reject all URIs")
	}
}

func TestValidateRedirectURI_SubstringNotMatched(t *testing.T) {
	client := &Client{
		RedirectURIs: []string{"https://app.example.com/callback"},
	}
	// A prefix/suffix should not match; only exact match.
	if ValidateRedirectURI(client, "https://app.example.com/callback/extra") {
		t.Error("expected substring to not match")
	}
	if ValidateRedirectURI(client, "https://app.example.com/callbac") {
		t.Error("expected partial URI to not match")
	}
}

// --- verifyPKCE ---

func TestVerifyPKCE_S256_Valid(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if !verifyPKCE(challenge, "S256", verifier) {
		t.Error("expected PKCE S256 verification to succeed")
	}
}

func TestVerifyPKCE_S256_Invalid(t *testing.T) {
	verifier := "correct-verifier"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if verifyPKCE(challenge, "S256", "wrong-verifier") {
		t.Error("expected PKCE verification with wrong verifier to fail")
	}
	_ = challenge
}

func TestVerifyPKCE_EmptyMethod_DefaultsToS256(t *testing.T) {
	verifier := "test-verifier-12345"
	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	if !verifyPKCE(challenge, "", verifier) {
		t.Error("expected empty method to default to S256 and succeed")
	}
}

func TestVerifyPKCE_UnsupportedMethod(t *testing.T) {
	if verifyPKCE("challenge", "plain", "challenge") {
		t.Error("expected unsupported method 'plain' to fail")
	}
}

// --- deriveKeyID ---

func TestDeriveKeyID_Deterministic(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid1 := deriveKeyID(key)
	kid2 := deriveKeyID(key)
	if kid1 != kid2 {
		t.Errorf("expected deterministic key ID, got %q and %q", kid1, kid2)
	}
	// Should be hex-encoded first 8 bytes of SHA-256 = 16 hex chars.
	if len(kid1) != 16 {
		t.Errorf("expected key ID length 16, got %d", len(kid1))
	}
}

func TestDeriveKeyID_DifferentKeys(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := rsa.GenerateKey(rand.Reader, 2048)
	if deriveKeyID(key1) == deriveKeyID(key2) {
		t.Error("expected different keys to produce different key IDs")
	}
}

// --- generateRandomCode ---

func TestGenerateRandomCode_Length(t *testing.T) {
	code, err := generateRandomCode(32)
	if err != nil {
		t.Fatal(err)
	}
	// 32 bytes -> 64 hex chars.
	if len(code) != 64 {
		t.Errorf("expected code length 64, got %d", len(code))
	}
}

func TestGenerateRandomCode_HexEncoded(t *testing.T) {
	code, err := generateRandomCode(16)
	if err != nil {
		t.Fatal(err)
	}
	_, err = hex.DecodeString(code)
	if err != nil {
		t.Errorf("expected valid hex, got error: %v", err)
	}
}

func TestGenerateRandomCode_Unique(t *testing.T) {
	code1, _ := generateRandomCode(32)
	code2, _ := generateRandomCode(32)
	if code1 == code2 {
		t.Error("expected two random codes to differ (extremely unlikely collision)")
	}
}

// --- base64URLEncode ---

func TestBase64URLEncode(t *testing.T) {
	data := []byte{0xff, 0xfe, 0xfd}
	encoded := base64URLEncode(data)
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(data) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(data))
	}
	for i := range data {
		if data[i] != decoded[i] {
			t.Errorf("byte %d: expected %x, got %x", i, data[i], decoded[i])
		}
	}
	// Should have no padding.
	if strings.Contains(encoded, "=") {
		t.Error("expected no padding in base64url encoding")
	}
}

// --- extractBearerToken ---

func TestExtractBearerToken_Valid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer my-secret-token")
	token := extractBearerToken(r)
	if token != "my-secret-token" {
		t.Errorf("expected %q, got %q", "my-secret-token", token)
	}
}

func TestExtractBearerToken_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	token := extractBearerToken(r)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestExtractBearerToken_WrongScheme(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	token := extractBearerToken(r)
	if token != "" {
		t.Errorf("expected empty token for Basic auth, got %q", token)
	}
}

func TestExtractBearerToken_EmptyToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	token := extractBearerToken(r)
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

// --- writeJSON ---

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"key":"value"`) {
		t.Errorf("unexpected body: %s", body)
	}
}

// --- tokenError ---

func TestTokenError(t *testing.T) {
	w := httptest.NewRecorder()
	tokenError(w, "invalid_grant", "code expired", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"invalid_grant"`) {
		t.Errorf("expected error code in body: %s", body)
	}
	if !strings.Contains(body, `"error_description":"code expired"`) {
		t.Errorf("expected error description in body: %s", body)
	}
}

// --- oidcError ---

func TestOidcError(t *testing.T) {
	w := httptest.NewRecorder()
	oidcError(w, http.StatusUnauthorized, "authentication required")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"authentication required"`) {
		t.Errorf("expected error in body: %s", body)
	}
	if !strings.Contains(body, `"message":"authentication required"`) {
		t.Errorf("expected message in body: %s", body)
	}
}

// --- IDTokenClaims as JWT ---

func TestIDTokenClaims_SignAndParse(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	user := UserInfo{
		UserID:        "user-1",
		Email:         "test@example.com",
		EmailVerified: true,
		Name:          "Test User",
		Username:      "testuser",
		Groups:        []string{"group1"},
	}
	claims := BuildIDTokenClaims(user, "https://iss", "client1", "n0nc3", 1*time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	// Parse it back.
	parsed, err := jwt.ParseWithClaims(tokenStr, &IDTokenClaims{}, func(t *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Valid {
		t.Error("expected token to be valid")
	}

	pc := parsed.Claims.(*IDTokenClaims)
	if pc.Subject != "user-1" {
		t.Errorf("expected subject user-1, got %q", pc.Subject)
	}
	if pc.Email != "test@example.com" {
		t.Errorf("expected email test@example.com, got %q", pc.Email)
	}
	if pc.Nonce != "n0nc3" {
		t.Errorf("expected nonce n0nc3, got %q", pc.Nonce)
	}
}

// --- AccessTokenClaims ---

func TestAccessTokenClaims_SignAndParse(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	claims := AccessTokenClaims{
		Scope:    "openid profile",
		ClientID: "client-abc",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://iss",
			Subject:   "user-99",
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := jwt.ParseWithClaims(tokenStr, &AccessTokenClaims{}, func(t *jwt.Token) (any, error) {
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	pc := parsed.Claims.(*AccessTokenClaims)
	if pc.Scope != "openid profile" {
		t.Errorf("expected scope %q, got %q", "openid profile", pc.Scope)
	}
	if pc.ClientID != "client-abc" {
		t.Errorf("expected client_id %q, got %q", "client-abc", pc.ClientID)
	}
	if pc.Subject != "user-99" {
		t.Errorf("expected subject %q, got %q", "user-99", pc.Subject)
	}
}

// --- errorRedirect ---

func TestErrorRedirect_WithState(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	errorRedirect(w, r, "https://app.example.com/callback", "state123", "invalid_request", "bad param")

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "error=invalid_request") {
		t.Errorf("expected error param in location: %s", loc)
	}
	if !strings.Contains(loc, "state=state123") {
		t.Errorf("expected state param in location: %s", loc)
	}
}

func TestErrorRedirect_EmptyRedirectURI(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	errorRedirect(w, r, "", "state", "error", "desc")

	// Should fall back to oidcError JSON response.
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty redirect, got %d", w.Code)
	}
}
