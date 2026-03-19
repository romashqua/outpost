package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// Session represents an authenticated user session.
type Session struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"`
	IPAddress string         `json:"ip_address"`
	UserAgent string         `json:"user_agent"`
	CreatedAt time.Time      `json:"created_at"`
	ExpiresAt time.Time      `json:"expires_at"`
	Data      map[string]any `json:"data,omitempty"`
}

// isExpired reports whether the session has passed its expiration time.
func (s *Session) isExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// Store defines the interface for session persistence. Implementations may be
// backed by memory, Redis, or any other storage layer.
type Store interface {
	Create(ctx context.Context, s *Session) error
	Get(ctx context.Context, id string) (*Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByUser(ctx context.Context, userID string) error
	List(ctx context.Context, userID string) ([]*Session, error)
	Touch(ctx context.Context, id string, ttl time.Duration) error
}

// ---- In-memory Store --------------------------------------------------------

// MemoryStore is a thread-safe in-memory implementation of Store.
type MemoryStore struct {
	m sync.Map
}

// NewMemoryStore creates a new MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (ms *MemoryStore) Create(_ context.Context, s *Session) error {
	ms.m.Store(s.ID, s)
	return nil
}

func (ms *MemoryStore) Get(_ context.Context, id string) (*Session, error) {
	v, ok := ms.m.Load(id)
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	s, ok := v.(*Session)
	if !ok {
		ms.m.Delete(id)
		return nil, fmt.Errorf("session %s: invalid type in store", id)
	}
	if s.isExpired() {
		ms.m.Delete(id)
		return nil, fmt.Errorf("session %s expired", id)
	}
	return s, nil
}

func (ms *MemoryStore) Delete(_ context.Context, id string) error {
	ms.m.Delete(id)
	return nil
}

func (ms *MemoryStore) DeleteByUser(_ context.Context, userID string) error {
	ms.m.Range(func(key, value any) bool {
		if s, ok := value.(*Session); ok && s.UserID == userID {
			ms.m.Delete(key)
		}
		return true
	})
	return nil
}

func (ms *MemoryStore) List(_ context.Context, userID string) ([]*Session, error) {
	var sessions []*Session
	now := time.Now()
	ms.m.Range(func(key, value any) bool {
		if s, ok := value.(*Session); ok && s.UserID == userID && now.Before(s.ExpiresAt) {
			sessions = append(sessions, s)
		}
		return true
	})
	return sessions, nil
}

func (ms *MemoryStore) Touch(_ context.Context, id string, ttl time.Duration) error {
	v, ok := ms.m.Load(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	old, ok := v.(*Session)
	if !ok {
		return fmt.Errorf("session %s: invalid type in store", id)
	}
	// Copy to avoid race with concurrent Get() calls.
	updated := *old
	updated.ExpiresAt = time.Now().Add(ttl)
	ms.m.Store(id, &updated)
	return nil
}

// ---- Manager ----------------------------------------------------------------

// Manager handles session lifecycle backed by a Store for fast access and
// PostgreSQL for durable persistence.
type Manager struct {
	store  Store
	pool   *pgxpool.Pool
	ttl    time.Duration
	logger *slog.Logger
}

// NewManager creates a session Manager.
func NewManager(store Store, pool *pgxpool.Pool, ttl time.Duration, logger *slog.Logger) *Manager {
	return &Manager{
		store:  store,
		pool:   pool,
		ttl:    ttl,
		logger: logger,
	}
}

// generateSessionID creates a cryptographically random 32-byte hex session ID.
func generateSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateSession creates a new session, persists it in the store and database.
func (m *Manager) CreateSession(ctx context.Context, userID, ipAddress, userAgent string) (*Session, error) {
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	s := &Session{
		ID:        id,
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		CreatedAt: now,
		ExpiresAt: now.Add(m.ttl),
		Data:      make(map[string]any),
	}

	// Write to store (fast path).
	if err := m.store.Create(ctx, s); err != nil {
		return nil, fmt.Errorf("store create: %w", err)
	}

	// Write to database (durable path).
	_, err = m.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, ip_address, user_agent, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		s.ID, s.UserID, s.IPAddress, s.UserAgent, s.CreatedAt, s.ExpiresAt,
	)
	if err != nil {
		m.logger.ErrorContext(ctx, "failed to persist session to DB", "error", err)
		// Session is still available in store; log but do not fail.
	}

	m.logger.InfoContext(ctx, "session created", "session_id", s.ID, "user_id", userID)
	return s, nil
}

// ValidateSession checks the store first, falling back to the database.
func (m *Manager) ValidateSession(ctx context.Context, sessionID string) (*Session, error) {
	// Try store first.
	s, err := m.store.Get(ctx, sessionID)
	if err == nil {
		return s, nil
	}

	// Fallback to database.
	s = &Session{}
	err = m.pool.QueryRow(ctx,
		`SELECT id, user_id, ip_address, user_agent, created_at, expires_at
		 FROM sessions
		 WHERE id = $1 AND expires_at > now()`,
		sessionID,
	).Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("session not found or expired")
	}

	// Repopulate the store for subsequent lookups.
	_ = m.store.Create(ctx, s)

	return s, nil
}

// RevokeSession deletes a single session from both store and database.
func (m *Manager) RevokeSession(ctx context.Context, sessionID string) error {
	_ = m.store.Delete(ctx, sessionID)

	_, err := m.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("deleting session from DB: %w", err)
	}

	m.logger.InfoContext(ctx, "session revoked", "session_id", sessionID)
	return nil
}

// RevokeAllSessions deletes all sessions for a given user.
func (m *Manager) RevokeAllSessions(ctx context.Context, userID string) error {
	_ = m.store.DeleteByUser(ctx, userID)

	_, err := m.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions from DB: %w", err)
	}

	m.logger.InfoContext(ctx, "all sessions revoked", "user_id", userID)
	return nil
}

// ListSessions returns all active sessions for a user from the database.
func (m *Manager) ListSessions(ctx context.Context, userID string) ([]*Session, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, user_id, ip_address, user_agent, created_at, expires_at
		 FROM sessions
		 WHERE user_id = $1 AND expires_at > now()
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.IPAddress, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []*Session{}
	}

	return sessions, nil
}

// ---- HTTP Handlers ----------------------------------------------------------

// Routes returns a chi.Router with session management endpoints.
// The router expects JWTMiddleware to have already been applied upstream.
func (m *Manager) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", m.listHandler)
	r.Delete("/", m.revokeAllHandler)
	r.Delete("/{id}", m.revokeHandler)
	return r
}

// @Summary List active sessions
// @Description Return all active sessions for the authenticated user.
// @Tags Sessions
// @Produce json
// @Success 200 {array} Session
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /sessions [get]
func (m *Manager) listHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	sessions, err := m.ListSessions(r.Context(), claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	respondJSON(w, http.StatusOK, sessions)
}

// @Summary Revoke a session
// @Description Revoke a specific session by ID. The session must belong to the authenticated user.
// @Tags Sessions
// @Param id path string true "Session ID"
// @Success 204 "Session revoked"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /sessions/{id} [delete]
func (m *Manager) revokeHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		respondError(w, http.StatusBadRequest, "session ID is required")
		return
	}

	// Verify the session belongs to the requesting user.
	s, err := m.ValidateSession(r.Context(), sessionID)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	if s.UserID != claims.UserID {
		respondError(w, http.StatusForbidden, "cannot revoke another user's session")
		return
	}

	if err := m.RevokeSession(r.Context(), sessionID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to revoke session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary Revoke all sessions
// @Description Revoke all active sessions for the authenticated user.
// @Tags Sessions
// @Success 204 "All sessions revoked"
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /sessions [delete]
func (m *Manager) revokeAllHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	if err := m.RevokeAllSessions(r.Context(), claims.UserID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---- Response helpers -------------------------------------------------------

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
