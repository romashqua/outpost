package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"

	"github.com/romashqua/outpost/internal/auth"
)

type AuthHandler struct {
	pool      *pgxpool.Pool
	jwtSecret string
}

func NewAuthHandler(pool *pgxpool.Pool, jwtSecret string) *AuthHandler {
	return &AuthHandler{pool: pool, jwtSecret: jwtSecret}
}

func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
	r.Post("/mfa/verify", h.verifyMFA)
	r.Post("/refresh", h.refreshToken)
	return r
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token       string `json:"token"`
	ExpiresAt   int64  `json:"expires_at"`
	MFARequired bool   `json:"mfa_required,omitempty"`
	MFAToken    string `json:"mfa_token,omitempty"`
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
		userID       string
		username     string
		email        string
		passwordHash string
		isAdmin      bool
		isActive     bool
	)

	err := h.pool.QueryRow(r.Context(),
		`SELECT id, username, email, password_hash, is_admin, is_active
		 FROM users WHERE username = $1`,
		req.Username,
	).Scan(&userID, &username, &email, &passwordHash, &isAdmin, &isActive)

	if err != nil {
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
	_ = h.pool.QueryRow(r.Context(),
		`SELECT mfa_enabled FROM users WHERE id = $1`, userID,
	).Scan(&mfaEnabled)

	if mfaEnabled {
		// Issue a short-lived MFA token instead of a full session token.
		mfaToken, err := auth.GenerateToken(h.jwtSecret, auth.TokenClaims{
			UserID:   userID,
			Username: username,
			Email:    email,
			IsAdmin:  false, // Limited token — no admin access until MFA verified.
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
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
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
	_ = h.pool.QueryRow(r.Context(),
		`SELECT is_admin FROM users WHERE id = $1`, claims.UserID,
	).Scan(&isAdmin)

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
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
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
			// Mark code as used.
			_, _ = h.pool.Exec(r.Context(),
				`UPDATE mfa_backup_codes SET used = true, used_at = now() WHERE id = $1`, id,
			)
			return true
		}
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
	if err != nil || !isActive {
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
