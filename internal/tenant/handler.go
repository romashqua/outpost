package tenant

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler provides HTTP endpoints for tenant management.
type Handler struct {
	mgr  *Manager
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewHandler creates a Handler backed by the given connection pool.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		mgr:  NewManager(pool),
		pool: pool,
		log:  logger.With("handler", "tenant"),
	}
}

// Routes returns a chi.Router with tenant CRUD endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Put("/", h.update)
		r.Delete("/", h.deactivate)
		r.Get("/stats", h.stats)
	})
	return r
}

type createTenantRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Plan        string `json:"plan"`
	MaxUsers    int    `json:"max_users"`
	MaxDevices  int    `json:"max_devices"`
	MaxNetworks int    `json:"max_networks"`
}

type updateTenantRequest struct {
	Name        *string `json:"name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Plan        *string `json:"plan,omitempty"`
	MaxUsers    *int    `json:"max_users,omitempty"`
	MaxDevices  *int    `json:"max_devices,omitempty"`
	MaxNetworks *int    `json:"max_networks,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
}

type tenantStats struct {
	TenantID     string `json:"tenant_id"`
	UserCount    int    `json:"user_count"`
	DeviceCount  int    `json:"device_count"`
	NetworkCount int    `json:"network_count"`
	GatewayCount int    `json:"gateway_count"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.mgr.List(r.Context())
	if err != nil {
		h.log.Error("failed to list tenants", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to list tenants", "message": "failed to list tenants",
		})
		return
	}
	respondJSON(w, http.StatusOK, tenants)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createTenantRequest
	if err := parseBody(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(), "message": err.Error(),
		})
		return
	}

	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "name is required", "message": "name is required",
		})
		return
	}
	if req.Slug == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "slug is required", "message": "slug is required",
		})
		return
	}

	validPlans := map[string]bool{"free": true, "pro": true, "enterprise": true}
	if req.Plan == "" {
		req.Plan = "free"
	}
	if !validPlans[req.Plan] {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "plan must be free, pro, or enterprise", "message": "plan must be free, pro, or enterprise",
		})
		return
	}

	// Defaults based on plan.
	if req.MaxUsers == 0 {
		switch req.Plan {
		case "free":
			req.MaxUsers = 10
		case "pro":
			req.MaxUsers = 100
		case "enterprise":
			req.MaxUsers = 10000
		}
	}
	if req.MaxDevices == 0 {
		switch req.Plan {
		case "free":
			req.MaxDevices = 20
		case "pro":
			req.MaxDevices = 500
		case "enterprise":
			req.MaxDevices = 50000
		}
	}
	if req.MaxNetworks == 0 {
		switch req.Plan {
		case "free":
			req.MaxNetworks = 2
		case "pro":
			req.MaxNetworks = 20
		case "enterprise":
			req.MaxNetworks = 200
		}
	}

	t := Tenant{
		Name:        req.Name,
		Slug:        req.Slug,
		Plan:        req.Plan,
		MaxUsers:    req.MaxUsers,
		MaxDevices:  req.MaxDevices,
		MaxNetworks: req.MaxNetworks,
		IsActive:    true,
	}

	created, err := h.mgr.Create(r.Context(), t)
	if err != nil {
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate") {
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": "tenant with this slug already exists", "message": "tenant with this slug already exists",
			})
			return
		}
		h.log.Error("failed to create tenant", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to create tenant", "message": "failed to create tenant",
		})
		return
	}

	h.log.Info("tenant created", "id", created.ID, "name", created.Name, "slug", created.Slug)
	respondJSON(w, http.StatusCreated, created)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id is required", "message": "id is required",
		})
		return
	}

	t, err := h.mgr.Get(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "tenant not found", "message": "tenant not found",
		})
		return
	}

	respondJSON(w, http.StatusOK, t)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id is required", "message": "id is required",
		})
		return
	}

	var req updateTenantRequest
	if err := parseBody(r, &req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(), "message": err.Error(),
		})
		return
	}

	// Validate plan if provided.
	if req.Plan != nil {
		validPlans := map[string]bool{"free": true, "pro": true, "enterprise": true}
		if !validPlans[*req.Plan] {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "plan must be free, pro, or enterprise", "message": "plan must be free, pro, or enterprise",
			})
			return
		}
	}

	// Fetch existing tenant.
	existing, err := h.mgr.Get(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "tenant not found", "message": "tenant not found",
		})
		return
	}

	// Apply partial updates.
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Slug != nil {
		existing.Slug = *req.Slug
	}
	if req.Plan != nil {
		existing.Plan = *req.Plan
	}
	if req.MaxUsers != nil {
		existing.MaxUsers = *req.MaxUsers
	}
	if req.MaxDevices != nil {
		existing.MaxDevices = *req.MaxDevices
	}
	if req.MaxNetworks != nil {
		existing.MaxNetworks = *req.MaxNetworks
	}
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := h.mgr.Update(r.Context(), *existing); err != nil {
		h.log.Error("failed to update tenant", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to update tenant", "message": "failed to update tenant",
		})
		return
	}

	// Refetch to return updated timestamps.
	updated, err := h.mgr.Get(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch updated tenant", "message": "failed to fetch updated tenant",
		})
		return
	}

	respondJSON(w, http.StatusOK, updated)
}

func (h *Handler) deactivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id is required", "message": "id is required",
		})
		return
	}

	// Soft-delete: set is_active = false.
	existing, err := h.mgr.Get(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "tenant not found", "message": "tenant not found",
		})
		return
	}

	existing.IsActive = false
	if err := h.mgr.Update(r.Context(), *existing); err != nil {
		h.log.Error("failed to deactivate tenant", "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to deactivate tenant", "message": "failed to deactivate tenant",
		})
		return
	}

	h.log.Info("tenant deactivated", "id", id, "name", existing.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "id is required", "message": "id is required",
		})
		return
	}

	// Verify tenant exists.
	if _, err := h.mgr.Get(r.Context(), id); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{
			"error": "tenant not found", "message": "tenant not found",
		})
		return
	}

	var s tenantStats
	s.TenantID = id

	ctx := r.Context()

	// Count users.
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(COUNT(*), 0) FROM users WHERE tenant_id = $1`, id,
	).Scan(&s.UserCount); err != nil {
		h.log.Error("failed to count tenant users", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load stats", "message": "failed to load stats",
		})
		return
	}

	// Count devices (through users since devices table has no tenant_id).
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(COUNT(*), 0) FROM devices d
		 WHERE d.user_id IN (SELECT id FROM users WHERE tenant_id = $1)`, id,
	).Scan(&s.DeviceCount); err != nil {
		h.log.Error("failed to count tenant devices", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load stats", "message": "failed to load stats",
		})
		return
	}

	// Count networks.
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(COUNT(*), 0) FROM networks WHERE tenant_id = $1`, id,
	).Scan(&s.NetworkCount); err != nil {
		h.log.Error("failed to count tenant networks", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load stats", "message": "failed to load stats",
		})
		return
	}

	// Count gateways.
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(COUNT(*), 0) FROM gateways WHERE tenant_id = $1`, id,
	).Scan(&s.GatewayCount); err != nil {
		h.log.Error("failed to count tenant gateways", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "failed to load stats", "message": "failed to load stats",
		})
		return
	}

	respondJSON(w, http.StatusOK, s)
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// parseBody decodes a JSON request body into dst.
func parseBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
