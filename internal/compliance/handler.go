package compliance

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler provides compliance API endpoints.
type Handler struct {
	checker *Checker
}

// NewHandler creates a compliance Handler backed by the given connection pool.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{
		checker: NewChecker(pool),
	}
}

// Routes returns a chi.Router with compliance endpoints mounted.
//
//	GET /report   - full compliance report
//	GET /soc2     - SOC2 checks
//	GET /iso27001 - ISO27001 checks
//	GET /gdpr     - GDPR checks
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/report", h.report)
	r.Get("/soc2", h.soc2)
	r.Get("/iso27001", h.iso27001)
	r.Get("/gdpr", h.gdpr)
	return r
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	report, err := h.checker.RunAllChecks(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to run compliance checks")
		return
	}
	respondJSON(w, http.StatusOK, report)
}

func (h *Handler) soc2(w http.ResponseWriter, r *http.Request) {
	checks, err := h.checker.RunSOC2Checks(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to run SOC2 checks")
		return
	}
	respondJSON(w, http.StatusOK, checks)
}

func (h *Handler) iso27001(w http.ResponseWriter, r *http.Request) {
	checks, err := h.checker.RunISO27001Checks(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to run ISO27001 checks")
		return
	}
	respondJSON(w, http.StatusOK, checks)
}

func (h *Handler) gdpr(w http.ResponseWriter, r *http.Request) {
	checks, err := h.checker.RunGDPRChecks(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to run GDPR checks")
		return
	}
	respondJSON(w, http.StatusOK, checks)
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
