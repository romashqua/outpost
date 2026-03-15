package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"

	"github.com/romashqua/outpost/internal/auth"
)

// PasswordResetMailer is the interface for sending password reset emails.
type PasswordResetMailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

type AuthHandler struct {
	pool      *pgxpool.Pool
	jwtSecret string
	log       *slog.Logger
	mailer    PasswordResetMailer
	baseURL   string // e.g. "https://vpn.example.com"
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

func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
	r.Post("/mfa/verify", h.verifyMFA)
	r.Post("/refresh", h.refreshToken)
	r.Post("/forgot-password", h.forgotPassword)
	r.Post("/reset-password", h.resetPassword)
	// change-password validates JWT internally via GetUserFromContext.
	r.With(auth.JWTMiddleware(h.jwtSecret)).Post("/change-password", h.changePassword)
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
	)

	err := h.pool.QueryRow(r.Context(),
		`SELECT id, username, email, password_hash, is_admin, is_active, password_must_change
		 FROM users WHERE username = $1`,
		req.Username,
	).Scan(&userID, &username, &email, &passwordHash, &isAdmin, &isActive, &passwordMustChange)

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

	if err := auth.CheckPassword(passwordHash, req.Password); err != nil {
		respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Check if user has MFA enabled.
	var mfaEnabled bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT mfa_enabled FROM users WHERE id = $1`, userID,
	).Scan(&mfaEnabled); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check MFA status")
		return
	}

	if mfaEnabled {
		// Issue a short-lived MFA token (10 min) instead of a full session token.
		mfaToken, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
			UserID:   userID,
			Username: username,
			Email:    email,
			IsAdmin:  false, // Limited token — no admin access until MFA verified.
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

	// Check user is still active.
	var isActive bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_active FROM users WHERE id = $1`, claims.UserID,
	).Scan(&isActive)
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
		IsAdmin:  claims.IsAdmin,
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

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	// For JWT-based auth, logout is client-side (discard token).
	// Server-side session invalidation will be added with Redis sessions.
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Forgot / Reset / Change Password ---

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

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
		h.log.Info("password reset token generated (no mailer configured)", "user_id", userID, "token", plainToken)
	}
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

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
	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	tokenHash := sha256.Sum256([]byte(req.Token))
	tokenHashHex := hex.EncodeToString(tokenHash[:])

	var tokenID, userID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, user_id FROM password_reset_tokens
		 WHERE token_hash = $1 AND used = false AND expires_at > now()`,
		tokenHashHex,
	).Scan(&tokenID, &userID)
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

	// Update password and clear force-change flag in one query.
	if _, err := h.pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, password_must_change = false, updated_at = now()
		 WHERE id = $2`, hash, userID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	// Mark token as used.
	_, _ = h.pool.Exec(r.Context(),
		`UPDATE password_reset_tokens SET used = true WHERE id = $1`, tokenID)

	h.log.Info("password reset completed", "user_id", userID)
	respondJSON(w, http.StatusOK, map[string]string{"message": "password has been reset"})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

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
	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusBadRequest, "new password must be at least 8 characters")
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
