package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua-labs/outpost/internal/auth"
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
	return r
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
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

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	// For JWT-based auth, logout is client-side (discard token).
	// Server-side session invalidation will be added with Redis sessions.
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
