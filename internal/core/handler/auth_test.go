package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romashqua/outpost/internal/auth"
)

// ---------- helpers ----------

const authTestJWTSecret = "test-secret-key-for-auth-tests"
const authTestPassword = "TestPass1!"

// authTestHash is a pre-computed bcrypt hash of authTestPassword.
var authTestHash string

func init() {
	h, err := auth.HashPassword(authTestPassword)
	if err != nil {
		panic(err)
	}
	authTestHash = h
}

func newTestAuthHandler(pool DB, opts ...func(*AuthHandler)) *AuthHandler {
	return NewAuthHandler(pool, authTestJWTSecret, opts...)
}

func authJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func authDecodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(rec.Body).Decode(dst))
}

// mockBlacklist implements auth.TokenBlacklist for testing.
type mockBlacklist struct {
	tokens map[string]time.Time
}

func newMockBlacklist() *mockBlacklist {
	return &mockBlacklist{tokens: make(map[string]time.Time)}
}

func (b *mockBlacklist) IsBlacklisted(_ context.Context, rawToken string) (bool, error) {
	_, ok := b.tokens[rawToken]
	return ok, nil
}

func (b *mockBlacklist) Add(_ context.Context, rawToken string, expiresAt time.Time) error {
	b.tokens[rawToken] = expiresAt
	return nil
}

// userRow returns columns matching the login SELECT query.
func authUserRow() []string {
	return []string{
		"id", "username", "email", "password_hash", "is_admin", "is_active",
		"password_must_change", "mfa_enabled", "failed_login_attempts", "locked_until",
	}
}

// ---------- Login tests ----------

func TestAuthLogin_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("admin").
		WillReturnRows(pgxmock.NewRows(authUserRow()).
			AddRow("uid-1", "admin", "admin@test.com", authTestHash, true, true, false, false, 0, nil))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "admin", Password: authTestPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp loginResponse
	authDecodeJSON(t, rec, &resp)
	assert.NotEmpty(t, resp.Token)
	assert.False(t, resp.MFARequired)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthLogin_InvalidCredentials(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("admin").
		WillReturnRows(pgxmock.NewRows(authUserRow()).
			AddRow("uid-1", "admin", "admin@test.com", authTestHash, true, true, false, false, 0, nil))

	// Expect failed_login_attempts increment.
	mock.ExpectExec(`UPDATE users SET failed_login_attempts`).
		WithArgs(1, "uid-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "admin", Password: "WrongPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid credentials", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthLogin_MissingFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)

	tests := []struct {
		name string
		body loginRequest
	}{
		{"missing username", loginRequest{Password: "pass"}},
		{"missing password", loginRequest{Username: "user"}},
		{"both missing", loginRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := authJSONBody(t, tt.body)
			req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
			rec := httptest.NewRecorder()
			h.login(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestAuthLogin_InactiveAccount(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("inactive").
		WillReturnRows(pgxmock.NewRows(authUserRow()).
			AddRow("uid-2", "inactive", "inactive@test.com", authTestHash, false, false, false, false, 0, nil))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "inactive", Password: authTestPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "account is disabled", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthLogin_LockedAccount(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	lockedUntil := time.Now().Add(10 * time.Minute)
	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("locked").
		WillReturnRows(pgxmock.NewRows(authUserRow()).
			AddRow("uid-3", "locked", "locked@test.com", authTestHash, false, true, false, false, 5, &lockedUntil))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "locked", Password: authTestPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Contains(t, resp["message"], "account is locked")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthLogin_MFARequired(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("mfauser").
		WillReturnRows(pgxmock.NewRows(authUserRow()).
			AddRow("uid-4", "mfauser", "mfa@test.com", authTestHash, false, true, false, true, 0, nil))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "mfauser", Password: authTestPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp loginResponse
	authDecodeJSON(t, rec, &resp)
	assert.True(t, resp.MFARequired)
	assert.NotEmpty(t, resp.MFAToken)
	assert.Empty(t, resp.Token)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthLogin_UserNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT .+ FROM users WHERE username = \$1`).
		WithArgs("nobody").
		WillReturnError(pgx.ErrNoRows)

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, loginRequest{Username: "nobody", Password: authTestPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	rec := httptest.NewRecorder()

	h.login(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------- MFA Verify tests ----------

func TestAuthVerifyMFA_InvalidCode(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Generate an MFA token.
	mfaToken, err := auth.GenerateToken(authTestJWTSecret, auth.TokenClaims{
		UserID:    "uid-4",
		Username:  "mfauser",
		Email:     "mfa@test.com",
		TokenType: "mfa",
	})
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT secret FROM mfa_totp WHERE user_id = \$1`).
		WithArgs("uid-4").
		WillReturnRows(pgxmock.NewRows([]string{"secret"}).
			AddRow([]byte("JBSWY3DPEHPK3PXP")))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, mfaVerifyRequest{MFAToken: mfaToken, Code: "000000", Method: "totp"})
	req := httptest.NewRequest(http.MethodPost, "/auth/mfa/verify", body)
	rec := httptest.NewRecorder()

	h.verifyMFA(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid mfa code", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthVerifyMFA_ExpiredToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// We test with a garbage token since GenerateToken sets ExpiresAt to 24h in the future.
	h := newTestAuthHandler(mock)
	body := authJSONBody(t, mfaVerifyRequest{MFAToken: "invalid.token.here", Code: "123456", Method: "totp"})
	req := httptest.NewRequest(http.MethodPost, "/auth/mfa/verify", body)
	rec := httptest.NewRecorder()

	h.verifyMFA(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid or expired mfa token", resp["message"])
}

func TestAuthVerifyMFA_MissingFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, mfaVerifyRequest{MFAToken: "", Code: ""})
	req := httptest.NewRequest(http.MethodPost, "/auth/mfa/verify", body)
	rec := httptest.NewRecorder()

	h.verifyMFA(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuthVerifyMFA_WrongTokenType(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	// Generate a regular session token (not MFA type).
	sessionToken, err := auth.GenerateToken(authTestJWTSecret, auth.TokenClaims{
		UserID:   "uid-1",
		Username: "admin",
		Email:    "admin@test.com",
		IsAdmin:  true,
	})
	require.NoError(t, err)

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, mfaVerifyRequest{MFAToken: sessionToken, Code: "123456", Method: "totp"})
	req := httptest.NewRequest(http.MethodPost, "/auth/mfa/verify", body)
	rec := httptest.NewRecorder()

	h.verifyMFA(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid mfa token type", resp["message"])
}

// ---------- Refresh Token tests ----------

func TestAuthRefreshToken_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	bl := newMockBlacklist()

	token, err := auth.GenerateToken(authTestJWTSecret, auth.TokenClaims{
		UserID:   "uid-1",
		Username: "admin",
		Email:    "admin@test.com",
		IsAdmin:  true,
	})
	require.NoError(t, err)

	mock.ExpectQuery(`SELECT is_active, is_admin FROM users WHERE id = \$1`).
		WithArgs("uid-1").
		WillReturnRows(pgxmock.NewRows([]string{"is_active", "is_admin"}).
			AddRow(true, true))

	h := newTestAuthHandler(mock, WithTokenBlacklist(bl))
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.refreshToken(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp loginResponse
	authDecodeJSON(t, rec, &resp)
	assert.NotEmpty(t, resp.Token)
	assert.Greater(t, resp.ExpiresAt, int64(0))
	// Old token should be blacklisted.
	assert.Contains(t, bl.tokens, token)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthRefreshToken_ExpiredToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer invalid.expired.token")
	rec := httptest.NewRecorder()

	h.refreshToken(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid or expired token", resp["message"])
}

func TestAuthRefreshToken_BlacklistedToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	bl := newMockBlacklist()

	token, err := auth.GenerateToken(authTestJWTSecret, auth.TokenClaims{
		UserID:   "uid-1",
		Username: "admin",
		Email:    "admin@test.com",
		IsAdmin:  true,
	})
	require.NoError(t, err)

	// Pre-blacklist the token.
	bl.tokens[token] = time.Now().Add(24 * time.Hour)

	h := newTestAuthHandler(mock, WithTokenBlacklist(bl))
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.refreshToken(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "token has been revoked", resp["message"])
}

func TestAuthRefreshToken_MissingHeader(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	rec := httptest.NewRecorder()

	h.refreshToken(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- Forgot Password tests ----------

func TestAuthForgotPassword_ValidEmail(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT id FROM users WHERE email = \$1 AND is_active = true`).
		WithArgs("admin@test.com").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).
			AddRow("uid-1"))

	mock.ExpectExec(`INSERT INTO password_reset_tokens`).
		WithArgs("uid-1", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, forgotPasswordRequest{Email: "admin@test.com"})
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", body)
	rec := httptest.NewRecorder()

	h.forgotPassword(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Contains(t, resp["message"], "if the email exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthForgotPassword_UnknownEmail(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT id FROM users WHERE email = \$1 AND is_active = true`).
		WithArgs("nobody@test.com").
		WillReturnError(pgx.ErrNoRows)

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, forgotPasswordRequest{Email: "nobody@test.com"})
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", body)
	rec := httptest.NewRecorder()

	h.forgotPassword(rec, req)

	// Should still return 200 for security (no email enumeration).
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Contains(t, resp["message"], "if the email exists")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthForgotPassword_MissingEmail(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, forgotPasswordRequest{Email: ""})
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", body)
	rec := httptest.NewRecorder()

	h.forgotPassword(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------- Reset Password tests ----------

func TestAuthResetPassword_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	plainToken := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	tokenHash := sha256.Sum256([]byte(plainToken))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	mock.ExpectQuery(`UPDATE password_reset_tokens SET used = true`).
		WithArgs(tokenHashHex).
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).
			AddRow("uid-1"))

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs(pgxmock.AnyArg(), "uid-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, resetPasswordRequest{Token: plainToken, NewPassword: "NewPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", body)
	rec := httptest.NewRecorder()

	h.resetPassword(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "password has been reset", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthResetPassword_InvalidToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`UPDATE password_reset_tokens SET used = true`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, resetPasswordRequest{Token: "bad-token", NewPassword: "NewPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", body)
	rec := httptest.NewRecorder()

	h.resetPassword(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "invalid or expired reset token", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthResetPassword_MissingFields(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, resetPasswordRequest{Token: "", NewPassword: ""})
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", body)
	rec := httptest.NewRecorder()

	h.resetPassword(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------- Change Password tests ----------

func TestAuthChangePassword_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT password_hash FROM users WHERE id = \$1`).
		WithArgs("uid-1").
		WillReturnRows(pgxmock.NewRows([]string{"password_hash"}).
			AddRow(authTestHash))

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs(pgxmock.AnyArg(), "uid-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, changePasswordRequest{CurrentPassword: authTestPassword, NewPassword: "NewPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", body)

	// Inject JWT claims into context (simulating JWTMiddleware).
	claims := &auth.TokenClaims{UserID: "uid-1", Username: "admin", Email: "admin@test.com", IsAdmin: true}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.changePassword(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "password changed successfully", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthChangePassword_WrongCurrentPassword(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT password_hash FROM users WHERE id = \$1`).
		WithArgs("uid-1").
		WillReturnRows(pgxmock.NewRows([]string{"password_hash"}).
			AddRow(authTestHash))

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, changePasswordRequest{CurrentPassword: "WrongPass1!", NewPassword: "NewPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", body)

	claims := &auth.TokenClaims{UserID: "uid-1", Username: "admin", Email: "admin@test.com", IsAdmin: true}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.changePassword(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "current password is incorrect", resp["message"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthChangePassword_Unauthenticated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	body := authJSONBody(t, changePasswordRequest{CurrentPassword: authTestPassword, NewPassword: "NewPass1!"})
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password", body)
	rec := httptest.NewRecorder()

	h.changePassword(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ---------- Logout tests ----------

func TestAuthLogout_AddsToBlacklist(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	bl := newMockBlacklist()

	token, err := auth.GenerateToken(authTestJWTSecret, auth.TokenClaims{
		UserID:   "uid-1",
		Username: "admin",
		Email:    "admin@test.com",
		IsAdmin:  true,
	})
	require.NoError(t, err)

	h := newTestAuthHandler(mock, WithTokenBlacklist(bl))
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	h.logout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "ok", resp["status"])
	// Token should now be in the blacklist.
	assert.Contains(t, bl.tokens, token)
}

func TestAuthLogout_NoToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := newTestAuthHandler(mock)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec := httptest.NewRecorder()

	h.logout(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]string
	authDecodeJSON(t, rec, &resp)
	assert.Equal(t, "ok", resp["status"])
}

func TestAuthLogout_InvalidToken(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	bl := newMockBlacklist()

	h := newTestAuthHandler(mock, WithTokenBlacklist(bl))
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()

	h.logout(rec, req)

	// Already invalid token — still returns OK.
	assert.Equal(t, http.StatusOK, rec.Code)
	// Should NOT be in blacklist since validation failed.
	assert.NotContains(t, bl.tokens, "invalid.token.here")
}
