package handler

import (
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// validHostnameRe matches a valid hostname label sequence (RFC 952 / RFC 1123).
var validHostnameRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

type SmartRouteHandler struct {
	pool *pgxpool.Pool
}

func NewSmartRouteHandler(pool *pgxpool.Pool) *SmartRouteHandler {
	return &SmartRouteHandler{pool: pool}
}

func (h *SmartRouteHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.listRoutes)
	r.With(auth.RequireAdmin).Post("/", h.createRoute)
	r.Get("/{id}", h.getRoute)
	r.With(auth.RequireAdmin).Put("/{id}", h.updateRoute)
	r.With(auth.RequireAdmin).Delete("/{id}", h.deleteRoute)
	r.With(auth.RequireAdmin).Post("/{id}/entries", h.addEntry)
	r.With(auth.RequireAdmin).Delete("/{id}/entries/{entryId}", h.deleteEntry)
	r.Get("/proxy-servers", h.listProxyServers)
	r.With(auth.RequireAdmin).Post("/proxy-servers", h.createProxyServer)
	r.Get("/proxy-servers/{id}", h.getProxyServer)
	r.With(auth.RequireAdmin).Put("/proxy-servers/{id}", h.updateProxyServer)
	r.With(auth.RequireAdmin).Delete("/proxy-servers/{id}", h.deleteProxyServer)
	r.Get("/{id}/networks", h.listRouteNetworks)
	r.With(auth.RequireAdmin).Post("/{id}/networks", h.addRouteNetwork)
	r.With(auth.RequireAdmin).Delete("/{id}/networks/{networkId}", h.removeRouteNetwork)
	return r
}

// --- Smart Route types ---

type smartRoute struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description *string            `json:"description,omitempty"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Entries     []smartRouteEntry  `json:"entries,omitempty"`
}

type smartRouteEntry struct {
	ID           string    `json:"id"`
	SmartRouteID string    `json:"smart_route_id"`
	EntryType    string    `json:"entry_type"`
	Value        string    `json:"value"`
	Action       string    `json:"action"`
	ProxyID      *string   `json:"proxy_id,omitempty"`
	ProxyName    *string   `json:"proxy_name,omitempty"`
	Priority     int       `json:"priority"`
	CreatedAt    time.Time `json:"created_at"`
}

type proxyServer struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Address     string    `json:"address"`
	Port        int       `json:"port"`
	Username    *string   `json:"username,omitempty"`
	Password    *string   `json:"password,omitempty"`
	ExtraConfig *string   `json:"extra_config,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// --- Smart Route CRUD ---

// @Summary List smart routes
// @Description Returns all smart routes.
// @Tags Smart Routes
// @Produce json
// @Success 200 {array} smartRoute
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes [get]
func (h *SmartRouteHandler) listRoutes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, description, is_active, created_at, updated_at
		 FROM smart_routes ORDER BY created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list smart routes")
		return
	}
	defer rows.Close()

	routes := make([]smartRoute, 0)
	for rows.Next() {
		var sr smartRoute
		if err := rows.Scan(&sr.ID, &sr.Name, &sr.Description, &sr.IsActive, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan smart route")
			return
		}
		routes = append(routes, sr)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate smart routes")
		return
	}

	respondJSON(w, http.StatusOK, routes)
}

// @Summary Create smart route
// @Description Create a new smart route. Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param body body object true "Smart route data (name, optional description)"
// @Success 201 {object} smartRoute
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes [post]
func (h *SmartRouteHandler) createRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Description *string `json:"description,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	var sr smartRoute
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO smart_routes (name, description)
		 VALUES ($1, $2)
		 RETURNING id, name, description, is_active, created_at, updated_at`,
		req.Name, req.Description,
	).Scan(&sr.ID, &sr.Name, &sr.Description, &sr.IsActive, &sr.CreatedAt, &sr.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "smart route with this name already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create smart route")
		return
	}

	respondJSON(w, http.StatusCreated, sr)
}

// @Summary Get smart route
// @Description Retrieve a smart route by ID with its entries.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Success 200 {object} smartRoute
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id} [get]
func (h *SmartRouteHandler) getRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var sr smartRoute
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, description, is_active, created_at, updated_at
		 FROM smart_routes WHERE id = $1`, id,
	).Scan(&sr.ID, &sr.Name, &sr.Description, &sr.IsActive, &sr.CreatedAt, &sr.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "smart route not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get smart route")
		return
	}

	// Fetch entries with proxy name.
	entryRows, err := h.pool.Query(r.Context(),
		`SELECT e.id, e.smart_route_id, e.entry_type, e.value, e.action, e.proxy_id, p.name, e.priority, e.created_at
		 FROM smart_route_entries e
		 LEFT JOIN proxy_servers p ON p.id = e.proxy_id
		 WHERE e.smart_route_id = $1
		 ORDER BY e.priority ASC, e.created_at ASC`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch entries")
		return
	}
	defer entryRows.Close()

	sr.Entries = make([]smartRouteEntry, 0)
	for entryRows.Next() {
		var e smartRouteEntry
		if err := entryRows.Scan(&e.ID, &e.SmartRouteID, &e.EntryType, &e.Value, &e.Action, &e.ProxyID, &e.ProxyName, &e.Priority, &e.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan entry")
			return
		}
		sr.Entries = append(sr.Entries, e)
	}
	if err := entryRows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate entries")
		return
	}

	respondJSON(w, http.StatusOK, sr)
}

// @Summary Update smart route
// @Description Update an existing smart route. Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Param body body object true "Fields to update (name, description, is_active)"
// @Success 200 {object} smartRoute
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id} [put]
func (h *SmartRouteHandler) updateRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var sr smartRoute
	err = h.pool.QueryRow(r.Context(),
		`UPDATE smart_routes
		 SET name = COALESCE($2, name),
		     description = COALESCE($3, description),
		     is_active = COALESCE($4, is_active),
		     updated_at = now()
		 WHERE id = $1
		 RETURNING id, name, description, is_active, created_at, updated_at`,
		id, req.Name, req.Description, req.IsActive,
	).Scan(&sr.ID, &sr.Name, &sr.Description, &sr.IsActive, &sr.CreatedAt, &sr.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "smart route with this name already exists")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "smart route not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update smart route")
		return
	}

	respondJSON(w, http.StatusOK, sr)
}

// @Summary Delete smart route
// @Description Delete a smart route by ID. Requires admin privileges.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id} [delete]
func (h *SmartRouteHandler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM smart_routes WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete smart route")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "smart route not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Smart Route Entries ---

// @Summary Add smart route entry
// @Description Add a routing entry (domain, CIDR, or domain_suffix) to a smart route. Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Param body body object true "Entry data (entry_type, value, action, optional proxy_id, priority)"
// @Success 201 {object} smartRouteEntry
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id}/entries [post]
func (h *SmartRouteHandler) addEntry(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		EntryType string  `json:"entry_type"`
		Value     string  `json:"value"`
		Action    string  `json:"action"`
		ProxyID   *string `json:"proxy_id,omitempty"`
		Priority  *int    `json:"priority,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.EntryType == "" || req.Value == "" || req.Action == "" {
		respondError(w, http.StatusBadRequest, "entry_type, value, and action are required")
		return
	}
	if req.EntryType != "domain" && req.EntryType != "cidr" && req.EntryType != "domain_suffix" {
		respondError(w, http.StatusBadRequest, "entry_type must be 'domain', 'cidr', or 'domain_suffix'")
		return
	}
	if req.Action != "proxy" && req.Action != "direct" && req.Action != "block" {
		respondError(w, http.StatusBadRequest, "action must be 'proxy', 'direct', or 'block'")
		return
	}
	if req.Action == "proxy" && req.ProxyID == nil {
		respondError(w, http.StatusBadRequest, "proxy_id is required when action is 'proxy'")
		return
	}

	// Validate entry value based on type.
	switch req.EntryType {
	case "cidr":
		if _, _, err := net.ParseCIDR(req.Value); err != nil {
			respondError(w, http.StatusBadRequest, "invalid CIDR format: "+req.Value)
			return
		}
	case "domain":
		if !validHostnameRe.MatchString(req.Value) {
			respondError(w, http.StatusBadRequest, "invalid domain: "+req.Value)
			return
		}
	case "domain_suffix":
		if !strings.HasPrefix(req.Value, ".") {
			respondError(w, http.StatusBadRequest, "domain_suffix must start with '.'")
			return
		}
		if !validHostnameRe.MatchString(req.Value[1:]) {
			respondError(w, http.StatusBadRequest, "invalid domain suffix: "+req.Value)
			return
		}
	}

	priority := 100
	if req.Priority != nil {
		if *req.Priority < 0 {
			respondError(w, http.StatusBadRequest, "priority must not be negative")
			return
		}
		priority = *req.Priority
	}

	var e smartRouteEntry
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO smart_route_entries (smart_route_id, entry_type, value, action, proxy_id, priority)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, smart_route_id, entry_type, value, action, proxy_id, priority, created_at`,
		routeID, req.EntryType, req.Value, req.Action, req.ProxyID, priority,
	).Scan(&e.ID, &e.SmartRouteID, &e.EntryType, &e.Value, &e.Action, &e.ProxyID, &e.Priority, &e.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				msg := "entry already exists"
				if strings.Contains(pgErr.ConstraintName, "smart_route_id") {
					msg = "duplicate entry for this route"
				}
				respondError(w, http.StatusConflict, msg)
				return
			case "23503":
				msg := "referenced resource not found"
				if strings.Contains(pgErr.ConstraintName, "proxy") {
					msg = "proxy server not found"
				} else if strings.Contains(pgErr.ConstraintName, "smart_route") {
					msg = "smart route not found"
				}
				respondError(w, http.StatusBadRequest, msg)
				return
			}
		}
		respondError(w, http.StatusInternalServerError, "failed to add entry")
		return
	}

	respondJSON(w, http.StatusCreated, e)
}

// @Summary Delete smart route entry
// @Description Remove an entry from a smart route. Requires admin privileges.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Param entryId path string true "Entry ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id}/entries/{entryId} [delete]
func (h *SmartRouteHandler) deleteEntry(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	entryID, err := parseUUID(r, "entryId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM smart_route_entries WHERE id = $1 AND smart_route_id = $2`, entryID, routeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete entry")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "entry not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Proxy Servers ---

// @Summary List proxy servers
// @Description Returns all proxy servers for smart routing.
// @Tags Smart Routes
// @Produce json
// @Success 200 {array} proxyServer
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/proxy-servers [get]
func (h *SmartRouteHandler) listProxyServers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at
		 FROM proxy_servers ORDER BY created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list proxy servers")
		return
	}
	defer rows.Close()

	servers := make([]proxyServer, 0)
	for rows.Next() {
		var ps proxyServer
		if err := rows.Scan(&ps.ID, &ps.Name, &ps.Type, &ps.Address, &ps.Port, &ps.Username, &ps.Password, &ps.ExtraConfig, &ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan proxy server")
			return
		}
		ps.Password = nil
		servers = append(servers, ps)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate proxy servers")
		return
	}

	respondJSON(w, http.StatusOK, servers)
}

// @Summary Create proxy server
// @Description Create a new proxy server (SOCKS5, HTTP, Shadowsocks, or VLESS). Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param body body object true "Proxy server data (name, type, address, port, optional credentials)"
// @Success 201 {object} proxyServer
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/proxy-servers [post]
func (h *SmartRouteHandler) createProxyServer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string  `json:"name"`
		Type        string  `json:"type"`
		Address     string  `json:"address"`
		Port        int     `json:"port"`
		Username    *string `json:"username,omitempty"`
		Password    *string `json:"password,omitempty"`
		ExtraConfig *string `json:"extra_config,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Type == "" || req.Address == "" || req.Port == 0 {
		respondError(w, http.StatusBadRequest, "name, type, address, and port are required")
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		respondError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.Type != "socks5" && req.Type != "http" && req.Type != "shadowsocks" && req.Type != "vless" {
		respondError(w, http.StatusBadRequest, "type must be 'socks5', 'http', 'shadowsocks', or 'vless'")
		return
	}

	var ps proxyServer
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO proxy_servers (name, type, address, port, username, password, extra_config)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		 RETURNING id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at`,
		req.Name, req.Type, req.Address, req.Port, req.Username, req.Password, req.ExtraConfig,
	).Scan(&ps.ID, &ps.Name, &ps.Type, &ps.Address, &ps.Port, &ps.Username, &ps.Password, &ps.ExtraConfig, &ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "proxy server already exists")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create proxy server")
		return
	}

	ps.Password = nil
	respondJSON(w, http.StatusCreated, ps)
}

// @Summary Get proxy server
// @Description Retrieve a proxy server by ID.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Proxy Server ID (UUID)"
// @Success 200 {object} proxyServer
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/proxy-servers/{id} [get]
func (h *SmartRouteHandler) getProxyServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var ps proxyServer
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at
		 FROM proxy_servers WHERE id = $1`, id,
	).Scan(&ps.ID, &ps.Name, &ps.Type, &ps.Address, &ps.Port, &ps.Username, &ps.Password, &ps.ExtraConfig, &ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "proxy server not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get proxy server")
		return
	}

	ps.Password = nil
	respondJSON(w, http.StatusOK, ps)
}

// @Summary Update proxy server
// @Description Update an existing proxy server. Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param id path string true "Proxy Server ID (UUID)"
// @Param body body object true "Fields to update"
// @Success 200 {object} proxyServer
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/proxy-servers/{id} [put]
func (h *SmartRouteHandler) updateProxyServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Name        *string `json:"name,omitempty"`
		Type        *string `json:"type,omitempty"`
		Address     *string `json:"address,omitempty"`
		Port        *int    `json:"port,omitempty"`
		Username    *string `json:"username,omitempty"`
		Password    *string `json:"password,omitempty"`
		ExtraConfig *string `json:"extra_config,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		respondError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.Type != nil && *req.Type != "socks5" && *req.Type != "http" && *req.Type != "shadowsocks" && *req.Type != "vless" {
		respondError(w, http.StatusBadRequest, "type must be 'socks5', 'http', 'shadowsocks', or 'vless'")
		return
	}

	var ps proxyServer
	err = h.pool.QueryRow(r.Context(),
		`UPDATE proxy_servers
		 SET name = COALESCE($2, name),
		     type = COALESCE($3, type),
		     address = COALESCE($4, address),
		     port = COALESCE($5, port),
		     username = COALESCE($6, username),
		     password = COALESCE($7, password),
		     extra_config = COALESCE($8::jsonb, extra_config),
		     is_active = COALESCE($9, is_active),
		     updated_at = now()
		 WHERE id = $1
		 RETURNING id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at`,
		id, req.Name, req.Type, req.Address, req.Port, req.Username, req.Password, req.ExtraConfig, req.IsActive,
	).Scan(&ps.ID, &ps.Name, &ps.Type, &ps.Address, &ps.Port, &ps.Username, &ps.Password, &ps.ExtraConfig, &ps.IsActive, &ps.CreatedAt, &ps.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "proxy server with this name already exists")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "proxy server not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update proxy server")
		return
	}

	ps.Password = nil
	respondJSON(w, http.StatusOK, ps)
}

// @Summary Delete proxy server
// @Description Delete a proxy server by ID. Fails if referenced by route entries. Requires admin privileges.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Proxy Server ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/proxy-servers/{id} [delete]
func (h *SmartRouteHandler) deleteProxyServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM proxy_servers WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusConflict, "proxy server is referenced by route entries")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete proxy server")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "proxy server not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Network ↔ Smart Route associations ---

type networkSmartRoute struct {
	NetworkID    string `json:"network_id"`
	SmartRouteID string `json:"smart_route_id"`
	NetworkName  string `json:"network_name"`
}

// @Summary List smart route networks
// @Description Returns all networks associated with a smart route.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Success 200 {array} networkSmartRoute
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id}/networks [get]
func (h *SmartRouteHandler) listRouteNetworks(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT nsr.network_id, nsr.smart_route_id, n.name
		 FROM network_smart_routes nsr
		 JOIN networks n ON n.id = nsr.network_id
		 WHERE nsr.smart_route_id = $1
		 ORDER BY n.name`, routeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list route networks")
		return
	}
	defer rows.Close()

	result := make([]networkSmartRoute, 0)
	for rows.Next() {
		var nsr networkSmartRoute
		if err := rows.Scan(&nsr.NetworkID, &nsr.SmartRouteID, &nsr.NetworkName); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan network association")
			return
		}
		result = append(result, nsr)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate network associations")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// @Summary Associate network with smart route
// @Description Associate a network with a smart route. Requires admin privileges.
// @Tags Smart Routes
// @Accept json
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Param body body object true "Network ID to associate"
// @Success 201 "Created"
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id}/networks [post]
func (h *SmartRouteHandler) addRouteNetwork(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		NetworkID string `json:"network_id"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.NetworkID == "" {
		respondError(w, http.StatusBadRequest, "network_id is required")
		return
	}
	if _, err := uuid.Parse(req.NetworkID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid network_id: not a valid UUID")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO network_smart_routes (network_id, smart_route_id) VALUES ($1, $2)`,
		req.NetworkID, routeID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				respondError(w, http.StatusConflict, "network is already associated with this route")
				return
			case "23503":
				msg := "referenced resource not found"
				if strings.Contains(pgErr.ConstraintName, "network") {
					msg = "network not found"
				} else if strings.Contains(pgErr.ConstraintName, "smart_route") {
					msg = "smart route not found"
				}
				respondError(w, http.StatusBadRequest, msg)
				return
			}
		}
		respondError(w, http.StatusInternalServerError, "failed to associate network")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// @Summary Remove network from smart route
// @Description Remove a network association from a smart route. Requires admin privileges.
// @Tags Smart Routes
// @Produce json
// @Param id path string true "Smart Route ID (UUID)"
// @Param networkId path string true "Network ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /smart-routes/{id}/networks/{networkId} [delete]
func (h *SmartRouteHandler) removeRouteNetwork(w http.ResponseWriter, r *http.Request) {
	routeID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	networkID, err := parseUUID(r, "networkId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM network_smart_routes WHERE network_id = $1 AND smart_route_id = $2`,
		networkID, routeID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to remove network association")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "association not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
