package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HealthHandler provides health check endpoints for liveness and readiness probes.
type HealthHandler struct {
	pool DB
}

// NewHealthHandler creates a HealthHandler. The pool may be nil if only
// liveness checks are needed.
func NewHealthHandler(pool DB) *HealthHandler {
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
// @Summary Liveness probe
// @Description Always returns 200 OK to indicate the service is alive.
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Router /healthz [get]
func (h *HealthHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz is a readiness probe that verifies database connectivity.
// @Summary Readiness probe
// @Description Verifies the database is reachable. Returns 503 if not.
// @Tags Health
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 503 {object} map[string]string
// @Router /readyz [get]
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
