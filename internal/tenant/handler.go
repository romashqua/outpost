package tenant

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
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
	r.With(auth.RequireAdmin).Get("/", h.list)
	r.With(auth.RequireAdmin).Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/", h.get)
		r.Put("/", h.update)
		r.Delete("/", h.deactivate)
		r.Get("/stats", h.stats)
		r.Get("/users", h.listUsers)
		r.Get("/networks", h.listNetworks)
		r.Get("/gateways", h.listGateways)
		r.Post("/users/{userId}", h.assignUser)
		r.Delete("/users/{userId}", h.unassignUser)
		r.Post("/networks/{networkId}", h.assignNetwork)
		r.Delete("/networks/{networkId}", h.unassignNetwork)
		r.Post("/gateways/{gatewayId}", h.assignGateway)
		r.Delete("/gateways/{gatewayId}", h.unassignGateway)
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

// @Summary List all tenants
// @Description Return all tenants. Requires admin role.
// @Tags Tenants
// @Produce json
// @Success 200 {array} Tenant
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants [get]
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

// @Summary Create a tenant
// @Description Create a new tenant with the given parameters. Requires admin role.
// @Tags Tenants
// @Accept json
// @Produce json
// @Param body body createTenantRequest true "Tenant creation parameters"
// @Success 201 {object} Tenant
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants [post]
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "tenant with this slug already exists"
			if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "tenant with this name already exists"
			}
			respondJSON(w, http.StatusConflict, map[string]string{
				"error": msg, "message": msg,
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

// @Summary Get a tenant
// @Description Return a single tenant by ID. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} Tenant
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id} [get]
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

// @Summary Update a tenant
// @Description Update an existing tenant by ID with partial fields. Requires admin role.
// @Tags Tenants
// @Accept json
// @Produce json
// @Param id path string true "Tenant ID"
// @Param body body updateTenantRequest true "Fields to update"
// @Success 200 {object} Tenant
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id} [put]
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

// @Summary Deactivate a tenant
// @Description Soft-delete a tenant by setting is_active to false. Requires admin role.
// @Tags Tenants
// @Param id path string true "Tenant ID"
// @Success 204 "Tenant deactivated"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id} [delete]
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

// @Summary Get tenant stats
// @Description Return usage statistics for a tenant (user, device, network, gateway counts). Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} tenantStats
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/stats [get]
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

// --- Tenant resource management endpoints ---

// @Summary List tenant users
// @Description Return all users assigned to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/users [get]
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.username, u.email, u.first_name, u.last_name,
		        CASE WHEN u.is_admin THEN 'admin' ELSE 'user' END AS role,
		        u.is_active, u.created_at
		 FROM users u
		 WHERE u.tenant_id = $1 ORDER BY u.created_at DESC`, id)
	if err != nil {
		h.log.Error("failed to query tenant users", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query users", "message": "failed to query users"})
		return
	}
	defer rows.Close()

	type userRow struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Role      string `json:"role"`
		IsActive  bool   `json:"is_active"`
		CreatedAt string `json:"created_at"`
	}
	users := make([]userRow, 0)
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &u.Role, &u.IsActive, &u.CreatedAt); err != nil {
			h.log.Error("failed to scan tenant user", "error", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan user", "message": "failed to scan user"})
			return
		}
		users = append(users, u)
	}
	respondJSON(w, http.StatusOK, users)
}

// @Summary List tenant networks
// @Description Return all networks assigned to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/networks [get]
func (h *Handler) listNetworks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, address::text, COALESCE(dns, '{}'), is_active, created_at
		 FROM networks WHERE tenant_id = $1 ORDER BY created_at DESC`, id)
	if err != nil {
		h.log.Error("failed to query tenant networks", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query networks", "message": "failed to query networks"})
		return
	}
	defer rows.Close()

	type networkRow struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		CIDR      string   `json:"cidr"`
		DNS       []string `json:"dns"`
		IsActive  bool     `json:"is_active"`
		CreatedAt string   `json:"created_at"`
	}
	networks := make([]networkRow, 0)
	for rows.Next() {
		var n networkRow
		if err := rows.Scan(&n.ID, &n.Name, &n.CIDR, &n.DNS, &n.IsActive, &n.CreatedAt); err != nil {
			h.log.Error("failed to scan tenant network", "tenant_id", id, "error", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan network", "message": "failed to scan network"})
			return
		}
		networks = append(networks, n)
	}
	respondJSON(w, http.StatusOK, networks)
}

// @Summary List tenant gateways
// @Description Return all gateways assigned to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/gateways [get]
func (h *Handler) listGateways(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, COALESCE(endpoint, ''), health_status,
		        last_seen IS NOT NULL AND last_seen > now() - interval '2 minutes' AS is_online,
		        created_at
		 FROM gateways WHERE tenant_id = $1 ORDER BY created_at DESC`, id)
	if err != nil {
		h.log.Error("failed to query tenant gateways", "tenant_id", id, "error", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query gateways", "message": "failed to query gateways"})
		return
	}
	defer rows.Close()

	type gatewayRow struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Endpoint     string `json:"endpoint"`
		HealthStatus string `json:"health_status"`
		IsOnline     bool   `json:"is_online"`
		CreatedAt    string `json:"created_at"`
	}
	gateways := make([]gatewayRow, 0)
	for rows.Next() {
		var g gatewayRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Endpoint, &g.HealthStatus, &g.IsOnline, &g.CreatedAt); err != nil {
			h.log.Error("failed to scan tenant gateway", "tenant_id", id, "error", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to scan gateway", "message": "failed to scan gateway"})
			return
		}
		gateways = append(gateways, g)
	}
	respondJSON(w, http.StatusOK, gateways)
}

// @Summary Assign user to tenant
// @Description Assign an existing user to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param userId path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/users/{userId} [post]
func (h *Handler) assignUser(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE users SET tenant_id = $1, updated_at = now() WHERE id = $2`, tenantID, userID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign user", "message": "failed to assign user"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found", "message": "user not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// @Summary Unassign user from tenant
// @Description Remove a user's tenant assignment. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param userId path string true "User ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/users/{userId} [delete]
func (h *Handler) unassignUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE users SET tenant_id = NULL, updated_at = now() WHERE id = $1`, userID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unassign user", "message": "failed to unassign user"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found", "message": "user not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

// @Summary Assign network to tenant
// @Description Assign an existing network to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param networkId path string true "Network ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/networks/{networkId} [post]
func (h *Handler) assignNetwork(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	networkID := chi.URLParam(r, "networkId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE networks SET tenant_id = $1, updated_at = now() WHERE id = $2`, tenantID, networkID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign network", "message": "failed to assign network"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "network not found", "message": "network not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// @Summary Unassign network from tenant
// @Description Remove a network's tenant assignment. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param networkId path string true "Network ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/networks/{networkId} [delete]
func (h *Handler) unassignNetwork(w http.ResponseWriter, r *http.Request) {
	networkID := chi.URLParam(r, "networkId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE networks SET tenant_id = NULL, updated_at = now() WHERE id = $1`, networkID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unassign network", "message": "failed to unassign network"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "network not found", "message": "network not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
}

// @Summary Assign gateway to tenant
// @Description Assign an existing gateway to a tenant. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param gatewayId path string true "Gateway ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/gateways/{gatewayId} [post]
func (h *Handler) assignGateway(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	gatewayID := chi.URLParam(r, "gatewayId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE gateways SET tenant_id = $1, updated_at = now() WHERE id = $2`, tenantID, gatewayID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign gateway", "message": "failed to assign gateway"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "gateway not found", "message": "gateway not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "assigned"})
}

// @Summary Unassign gateway from tenant
// @Description Remove a gateway's tenant assignment. Requires admin role.
// @Tags Tenants
// @Produce json
// @Param id path string true "Tenant ID"
// @Param gatewayId path string true "Gateway ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /tenants/{id}/gateways/{gatewayId} [delete]
func (h *Handler) unassignGateway(w http.ResponseWriter, r *http.Request) {
	gatewayID := chi.URLParam(r, "gatewayId")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE gateways SET tenant_id = NULL, updated_at = now() WHERE id = $1`, gatewayID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to unassign gateway", "message": "failed to unassign gateway"})
		return
	}
	if tag.RowsAffected() == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "gateway not found", "message": "gateway not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "unassigned"})
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
	limited := io.LimitReader(r.Body, 1<<20) // 1MB limit
	dec := json.NewDecoder(limited)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
