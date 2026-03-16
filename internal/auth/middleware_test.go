package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// mockBlacklist implements TokenBlacklist for testing.
type mockBlacklist struct {
	revoked map[string]bool
}

func (m *mockBlacklist) Add(_ context.Context, token string, _ time.Time) error {
	m.revoked[token] = true
	return nil
}

func (m *mockBlacklist) IsBlacklisted(_ context.Context, token string) (bool, error) {
	return m.revoked[token], nil
}

func (m *mockBlacklist) Cleanup(_ context.Context) error { return nil }

func makeToken(t *testing.T, secret string, claims TokenClaims) string {
	t.Helper()
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(10 * time.Minute))
	}
	token, err := GenerateToken(secret, claims)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return token
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	mw := JWTMiddleware("secret")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_InvalidFormat(t *testing.T) {
	mw := JWTMiddleware("secret")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic abc")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	mw := JWTMiddleware("secret")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret-key-for-jwt"
	token := makeToken(t, secret, TokenClaims{
		UserID:  "user-1",
		IsAdmin: true,
	})

	var gotClaims *TokenClaims
	mw := JWTMiddleware(secret)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			t.Error("expected claims in context")
		}
		gotClaims = claims
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotClaims == nil {
		t.Fatal("claims should not be nil")
	}
	if gotClaims.UserID != "user-1" {
		t.Errorf("expected user-1, got %s", gotClaims.UserID)
	}
	if !gotClaims.IsAdmin {
		t.Error("expected IsAdmin = true")
	}
}

func TestJWTMiddleware_WrongSecret(t *testing.T) {
	token := makeToken(t, "secret-A", TokenClaims{UserID: "user-1"})

	mw := JWTMiddleware("secret-B")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_BlacklistedToken(t *testing.T) {
	secret := "test-secret"
	token := makeToken(t, secret, TokenClaims{UserID: "user-1", IsAdmin: true})

	bl := &mockBlacklist{revoked: map[string]bool{token: true}}
	mw := JWTMiddleware(secret, bl)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for blacklisted token")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestJWTMiddleware_MFATokenRejected(t *testing.T) {
	secret := "test-secret"
	token := makeToken(t, secret, TokenClaims{
		UserID:    "user-1",
		TokenType: "mfa",
	})

	mw := JWTMiddleware(secret)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for MFA token")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAdmin_AdminAllowed(t *testing.T) {
	claims := &TokenClaims{UserID: "admin-1", Roles: []string{"admin"}, IsAdmin: true}
	ctx := context.WithValue(context.Background(), claimsKey, claims)

	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAdmin_NonAdminForbidden(t *testing.T) {
	claims := &TokenClaims{UserID: "user-1", Roles: []string{"user"}, IsAdmin: false}
	ctx := context.WithValue(context.Background(), claimsKey, claims)

	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireAdmin_NoClaimsUnauthorized(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestGetUserFromContext_Empty(t *testing.T) {
	_, ok := GetUserFromContext(context.Background())
	if ok {
		t.Error("expected no claims in empty context")
	}
}
