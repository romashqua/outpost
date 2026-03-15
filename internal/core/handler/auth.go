package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"

	"github.com/romashqua/outpost/internal/auth"
)

// PasswordResetMailer is the interface for sending password reset emails.
type PasswordResetMailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

type AuthHandler struct {
	pool           *pgxpool.Pool
	jwtSecret      string
	log            *slog.Logger
	mailer         PasswordResetMailer
	baseURL        string // e.g. "https://vpn.example.com"
	tokenBlacklist auth.TokenBlacklist
}

func NewAuthHandler(pool *pgxpool.Pool, jwtSecret string, opts ...func(*AuthHandler)) *AuthHandler {
	h := &AuthHandler{pool: pool, jwtSecret: jwtSecret, log: slog.Default()}
	for _, o := range opts {
		o(h)
	}
	return h
}

func WithAuthLogger(l *slog.Logger) func(*AuthHandler) {
	return func(h *AuthHandler) { h.log = l }
}

func WithAuthMailer(m PasswordResetMailer) func(*AuthHandler) {
	return func(h *AuthHandler) { h.mailer = m }
}

func WithBaseURL(u string) func(*AuthHandler) {
	return func(h *AuthHandler) { h.baseURL = u }
}

func WithTokenBlacklist(bl auth.TokenBlacklist) func(*AuthHandler) {
	return func(h *AuthHandler) { h.tokenBlacklist = bl }
}

func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
	r.Post("/mfa/verify", h.verifyMFA)
	r.Post("/refresh", h.refreshToken)
	r.Post("/forgot-password", h.forgotPassword)
	r.Post("/reset-password", h.resetPassword)
	// change-password validates JWT internally via GetUserFromContext.
	r.With(auth.JWTMiddleware(h.jwtSecret, h.tokenBlacklist)).Post("/change-password", h.changePassword)
	return r
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token              string `json:"token"`
	ExpiresAt          int64  `json:"expires_at"`
	MFARequired        bool   `json:"mfa_required,omitempty"`
	MFAToken           string `json:"mfa_token,omitempty"`
	PasswordMustChange bool   `json:"password_must_change,omitempty"`
}

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
	Method   string `json:"method"` // totp, email, backup
}

// @Summary Login
// @Description Authenticate a user with username and password. Returns JWT token or MFA challenge.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "Login credentials"
// @Success 200 {object} loginResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/login [post]
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	var (
		userID             string
		username           string
		email              string
		passwordHash       string
		isAdmin            bool
		isActive           bool
		passwordMustChange bool
		mfaEnabled         bool
		failedAttempts     int
		lockedUntil        *time.Time
	)

	err := h.pool.QueryRow(r.Context(),
		`SELECT id, username, email, password_hash, is_admin, is_active, password_must_change,
		        mfa_enabled, failed_login_attempts, locked_until
		 FROM users WHERE username = $1`,
		req.Username,
	).Scan(&userID, &username, &email, &passwordHash, &isAdmin, &isActive, &passwordMustChange,
		&mfaEnabled, &failedAttempts, &lockedUntil)

	if err != nil {
		// Constant-time: always run bcrypt to prevent timing-based user enumeration.
		_ = auth.CheckPassword("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl0UhmRMM7pNmuj9iesVaFnHa", req.Password)
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !isActive {
		respondError(w, http.StatusForbidden, "account is disabled")
		return
	}

	// Check account lockout.
	if lockedUntil != nil && lockedUntil.After(time.Now()) {
		remaining := time.Until(*lockedUntil).Truncate(time.Second)
		respondError(w, http.StatusForbidden, fmt.Sprintf("account is locked, try again after %s", remaining))
		return
	}

	if err := auth.CheckPassword(passwordHash, req.Password); err != nil {
		// Increment failed attempts; lock account after 5 consecutive failures.
		newAttempts := failedAttempts + 1
		if newAttempts >= 5 {
			lockUntil := time.Now().Add(15 * time.Minute)
			_, _ = h.pool.Exec(r.Context(),
				`UPDATE users SET failed_login_attempts = $1, locked_until = $2 WHERE id = $3`,
				newAttempts, lockUntil, userID)
			h.log.Warn("account locked due to failed login attempts", "user_id", userID, "attempts", newAttempts)
		} else {
			_, _ = h.pool.Exec(r.Context(),
				`UPDATE users SET failed_login_attempts = $1 WHERE id = $2`,
				newAttempts, userID)
		}
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Successful password check — reset lockout counters.
	if failedAttempts > 0 || lockedUntil != nil {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = $1`,
			userID)
	}

	if mfaEnabled {
		// Issue a short-lived MFA token (10 min) instead of a full session token.
		mfaToken, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
			UserID:    userID,
			Username:  username,
			Email:     email,
			IsAdmin:   false, // Limited token — no admin access until MFA verified.
			TokenType: "mfa",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
			},
		})
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate mfa token")
			return
		}
		respondJSON(w, http.StatusOK, loginResponse{
			MFARequired: true,
			MFAToken:    mfaToken,
		})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
		UserID:   userID,
		Username: username,
		Email:    email,
		IsAdmin:  isAdmin,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, loginResponse{
		Token:              token,
		ExpiresAt:          expiresAt.Unix(),
		PasswordMustChange: passwordMustChange,
	})
}

// @Summary Verify MFA
// @Description Verify a multi-factor authentication code (TOTP or backup) to complete login.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body mfaVerifyRequest true "MFA verification payload"
// @Success 200 {object} loginResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/mfa/verify [post]
func (h *AuthHandler) verifyMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.MFAToken == "" || req.Code == "" {
		respondError(w, http.StatusBadRequest, "mfa_token and code are required")
		return
	}

	// Validate the MFA token to extract user info.
	claims, err := auth.ValidateToken(h.jwtSecret, req.MFAToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid or expired mfa token")
		return
	}

	// Ensure this is actually an MFA-pending token, not a regular session token.
	if claims.TokenType != "mfa" {
		respondError(w, http.StatusUnauthorized, "invalid mfa token type")
		return
	}

	// Verify the TOTP/backup code against the database.
	var totpSecret string
	err = h.pool.QueryRow(r.Context(),
		`SELECT secret FROM mfa_totp WHERE user_id = $1`, claims.UserID,
	).Scan(&totpSecret)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "mfa not configured")
		return
	}

	valid := totp.Validate(req.Code, totpSecret)

	// If TOTP failed, try backup code.
	if !valid && (req.Method == "backup" || len(req.Code) == 8) {
		valid = h.tryBackupCode(r, claims.UserID, req.Code)
	}

	if !valid {
		respondError(w, http.StatusUnauthorized, "invalid mfa code")
		return
	}

	// Re-read full user info for the full session token.
	var isAdmin bool
	var pwdMustChange bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT is_admin, password_must_change FROM users WHERE id = $1`, claims.UserID,
	).Scan(&isAdmin, &pwdMustChange); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read user info")
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
		UserID:   claims.UserID,
		Username: claims.Username,
		Email:    claims.Email,
		IsAdmin:  isAdmin,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, loginResponse{
		Token:              token,
		ExpiresAt:          expiresAt.Unix(),
		PasswordMustChange: pwdMustChange,
	})
}

func (h *AuthHandler) tryBackupCode(r *http.Request, userID, code string) bool {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, code_hash FROM mfa_backup_codes WHERE user_id = $1 AND used = false`, userID,
	)
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var id, codeHash string
		if err := rows.Scan(&id, &codeHash); err != nil {
			continue
		}
		if auth.CheckPassword(codeHash, code) == nil {
			// Atomically mark code as used to prevent race conditions.
			// Only succeeds if the code hasn't been used between SELECT and UPDATE.
			tag, err := h.pool.Exec(r.Context(),
				`UPDATE mfa_backup_codes SET used = true WHERE id = $1 AND used = false`, id,
			)
			if err != nil || tag.RowsAffected() == 0 {
				continue // Code was used by another concurrent request.
			}
			return true
		}
	}
	if err := rows.Err(); err != nil {
		return false
	}
	return false
}

// @Summary Refresh token
// @Description Refresh an existing JWT token. The current token must be valid.
// @Tags Auth
// @Produce json
// @Success 200 {object} loginResponse
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /auth/refresh [post]
func (h *AuthHandler) refreshToken(w http.ResponseWriter, r *http.Request) {
	// Extract current token from Authorization header.
	tokenStr := r.Header.Get("Authorization")
	if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
		tokenStr = tokenStr[7:]
	} else {
		respondError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}

	claims, err := auth.ValidateToken(h.jwtSecret, tokenStr)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	// MFA-pending tokens cannot be refreshed into full session tokens.
	if claims.TokenType == "mfa" {
		respondError(w, http.StatusUnauthorized, "mfa verification required")
		return
	}

	// Re-read current user state from DB (is_active AND is_admin may have changed).
	var isActive, isAdmin bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_active, is_admin FROM users WHERE id = $1`, claims.UserID,
	).Scan(&isActive, &isAdmin)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify account status")
		return
	}
	if !isActive {
		respondError(w, http.StatusForbidden, "account is disabled")
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	newToken, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
		UserID:   claims.UserID,
		Username: claims.Username,
		Email:    claims.Email,
		IsAdmin:  isAdmin,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	respondJSON(w, http.StatusOK, loginResponse{
		Token:     newToken,
		ExpiresAt: expiresAt.Unix(),
	})
}

// @Summary Logout
// @Description Logout the current user. Client should discard the token.
// @Tags Auth
// @Produce json
// @Success 200 {object} map[string]string
// @Router /auth/logout [post]
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	tokenStr, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || tokenStr == "" {
		// No token provided — nothing to invalidate.
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Validate to extract expiry, then blacklist.
	claims, err := auth.ValidateToken(h.jwtSecret, tokenStr)
	if err != nil {
		// Token is already invalid/expired — nothing to revoke.
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	if h.tokenBlacklist != nil && claims.ExpiresAt != nil {
		if err := h.tokenBlacklist.Add(r.Context(), tokenStr, claims.ExpiresAt.Time); err != nil {
			h.log.Error("failed to blacklist token on logout", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to invalidate token")
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Forgot / Reset / Change Password ---

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// @Summary Forgot password
// @Description Request a password reset email. Always returns success to prevent email enumeration.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body forgotPasswordRequest true "Email address"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /auth/forgot-password [post]
func (h *AuthHandler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		respondError(w, http.StatusBadRequest, "email is required")
		return
	}

	// Always return success to prevent email enumeration.
	defer respondJSON(w, http.StatusOK, map[string]string{
		"message": "if the email exists, a reset link has been sent",
	})

	var userID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1 AND is_active = true`, req.Email,
	).Scan(&userID)
	if err != nil {
		return // user not found — silent success
	}

	// Generate a cryptographic reset token.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		h.log.Error("failed to generate reset token", "error", err)
		return
	}
	plainToken := hex.EncodeToString(tokenBytes)
	tokenHash := sha256.Sum256([]byte(plainToken))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	expiresAt := time.Now().Add(1 * time.Hour)
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHashHex, expiresAt,
	); err != nil {
		h.log.Error("failed to store reset token", "error", err)
		return
	}

	// Send email if mailer is configured.
	if h.mailer != nil {
		resetURL := h.baseURL + "/login?reset_token=" + plainToken
		go func() {
			if err := h.mailer.SendPasswordReset(context.Background(), req.Email, resetURL); err != nil {
				h.log.Error("failed to send password reset email", "email", req.Email, "error", err)
			}
		}()
	} else {
		h.log.Info("password reset token generated (no mailer configured)", "user_id", userID)
	}
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// @Summary Reset password
// @Description Reset password using a valid reset token.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body resetPasswordRequest true "Reset token and new password"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /auth/reset-password [post]
func (h *AuthHandler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "token and new_password are required")
		return
	}
	if err := auth.ValidatePasswordPolicy(req.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tokenHash := sha256.Sum256([]byte(req.Token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	// Atomically consume the token to prevent TOCTOU race conditions.
	var userID string
	err := h.pool.QueryRow(r.Context(),
		`UPDATE password_reset_tokens SET used = true
		 WHERE token_hash = $1 AND used = false AND expires_at > now()
		 RETURNING user_id`,
		tokenHashHex,
	).Scan(&userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, http.StatusBadRequest, "invalid or expired reset token")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to validate token")
		}
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	// Update password and clear force-change flag.
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, password_must_change = false, updated_at = now()
		 WHERE id = $2`, hash, userID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	h.log.Info("password reset completed", "user_id", userID)
	respondJSON(w, http.StatusOK, map[string]string{"message": "password has been reset"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// @Summary Change password
// @Description Change the current user's password. Requires valid current password.
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body changePasswordRequest true "Current and new password"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /auth/change-password [post]
func (h *AuthHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	// This endpoint requires a valid JWT (the user must be logged in).
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req changePasswordRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	if err := auth.ValidatePasswordPolicy(req.NewPassword); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify current password.
	var passwordHash string
	err := h.pool.QueryRow(r.Context(),
		`SELECT password_hash FROM users WHERE id = $1`, claims.UserID,
	).Scan(&passwordHash)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read user")
		return
	}

	if err := auth.CheckPassword(passwordHash, req.CurrentPassword); err != nil {
		respondError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if _, err := h.pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, password_must_change = false, updated_at = now()
		 WHERE id = $2`, newHash, claims.UserID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	h.log.Info("password changed", "user_id", claims.UserID)
	respondJSON(w, http.StatusOK, map[string]string{"message": "password changed successfully"})
}
