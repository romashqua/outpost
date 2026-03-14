package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HealthHandler provides health check endpoints for liveness and readiness probes.
type HealthHandler struct {
	pool *pgxpool.Pool
}

// NewHealthHandler creates a HealthHandler. The pool may be nil if only
// liveness checks are needed.
func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// Routes returns a chi.Router with health check endpoints mounted.
func (h *HealthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/healthz", h.Healthz)
	r.Get("/readyz", h.Readyz)
	return r
}

// Healthz is a liveness probe that always returns 200.
func (h *HealthHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz is a readiness probe that verifies database connectivity.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		respondError(w, http.StatusServiceUnavailable, "database pool not configured")
		return
	}

	if err := h.pool.Ping(r.Context()); err != nil {
		respondError(w, http.StatusServiceUnavailable, "database unreachable")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
