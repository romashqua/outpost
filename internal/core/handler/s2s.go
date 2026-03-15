package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// S2SNotifier pushes S2S config updates to connected gateways.
type S2SNotifier interface {
	NotifyS2SUpdate(gatewayID string, tunnelID string, action string)
}

type S2SHandler struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	notifier S2SNotifier
}

func NewS2SHandler(pool *pgxpool.Pool, notifier S2SNotifier, logger ...*slog.Logger) *S2SHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &S2SHandler{pool: pool, log: l.With("handler", "s2s"), notifier: notifier}
}

func (h *S2SHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.With(auth.RequireAdmin).Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.With(auth.RequireAdmin).Delete("/", h.delete)
		r.Get("/members", h.listMembers)
		r.With(auth.RequireAdmin).Post("/members", h.addMember)
		r.With(auth.RequireAdmin).Delete("/members/{gatewayId}", h.removeMember)
		r.Get("/routes", h.listRoutes)
		r.With(auth.RequireAdmin).Post("/routes", h.addRoute)
		r.With(auth.RequireAdmin).Delete("/routes/{routeId}", h.removeRoute)
		r.Get("/config/{gatewayId}", h.generateConfig)
		r.Get("/domains", h.listDomains)
		r.With(auth.RequireAdmin).Post("/domains", h.addDomain)
		r.With(auth.RequireAdmin).Delete("/domains/{domainId}", h.removeDomain)
	})
	return r
}

type s2sTunnel struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Topology     string    `json:"topology"`
	HubGatewayID *string   `json:"hub_gateway_id,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// @Summary List S2S tunnels
// @Description Returns all site-to-site tunnels.
// @Tags S2S Tunnels
// @Produce json
// @Success 200 {array} s2sTunnel
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels [get]
func (h *S2SHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, COALESCE(description, ''), topology, hub_gateway_id, is_active, created_at, updated_at
		 FROM s2s_tunnels ORDER BY created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list tunnels")
		return
	}
	defer rows.Close()

	tunnels := make([]s2sTunnel, 0)
	for rows.Next() {
		var t s2sTunnel
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Topology, &t.HubGatewayID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan tunnel")
			return
		}
		tunnels = append(tunnels, t)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate tunnels")
		return
	}

	respondJSON(w, http.StatusOK, tunnels)
}

// @Summary Create S2S tunnel
// @Description Create a new site-to-site tunnel (mesh or hub-spoke). Requires admin privileges.
// @Tags S2S Tunnels
// @Accept json
// @Produce json
// @Param body body object true "Tunnel data (name, topology, optional hub_gateway_id)"
// @Success 201 {object} s2sTunnel
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels [post]
func (h *S2SHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Topology     string  `json:"topology"`
		HubGatewayID *string `json:"hub_gateway_id,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Topology == "" {
		respondError(w, http.StatusBadRequest, "name and topology are required")
		return
	}
	if req.Topology != "mesh" && req.Topology != "hub_spoke" {
		respondError(w, http.StatusBadRequest, "topology must be 'mesh' or 'hub_spoke'")
		return
	}

	var t s2sTunnel
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO s2s_tunnels (name, description, topology, hub_gateway_id)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, COALESCE(description, ''), topology, hub_gateway_id, is_active, created_at, updated_at`,
		req.Name, req.Description, req.Topology, req.HubGatewayID,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Topology, &t.HubGatewayID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "tunnel already exists"
			if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "tunnel with this name already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create tunnel")
		return
	}

	h.log.Info("s2s tunnel created", "id", t.ID, "name", t.Name, "topology", t.Topology)
	respondJSON(w, http.StatusCreated, t)
}

// @Summary Get S2S tunnel
// @Description Retrieve a site-to-site tunnel by ID.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Success 200 {object} s2sTunnel
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id} [get]
func (h *S2SHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var t s2sTunnel
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, COALESCE(description, ''), topology, hub_gateway_id, is_active, created_at, updated_at
		 FROM s2s_tunnels WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Description, &t.Topology, &t.HubGatewayID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "tunnel not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get tunnel")
		return
	}

	respondJSON(w, http.StatusOK, t)
}

// @Summary Delete S2S tunnel
// @Description Delete a site-to-site tunnel by ID. Requires admin privileges.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id} [delete]
func (h *S2SHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM s2s_tunnels WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete tunnel")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "tunnel not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Members ---

type s2sMember struct {
	TunnelID     string   `json:"tunnel_id"`
	GatewayID    string   `json:"gateway_id"`
	GatewayName  string   `json:"gateway_name"`
	LocalSubnets []string `json:"local_subnets"`
}

// @Summary List S2S tunnel members
// @Description Returns all gateway members of a tunnel with their local subnets.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Success 200 {array} s2sMember
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/members [get]
func (h *S2SHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.tunnel_id, m.gateway_id, g.name,
		        ARRAY(SELECT unnest(m.local_subnets)::text)
		 FROM s2s_tunnel_members m
		 JOIN gateways g ON g.id = m.gateway_id
		 WHERE m.tunnel_id = $1`, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	defer rows.Close()

	members := make([]s2sMember, 0)
	for rows.Next() {
		var m s2sMember
		if err := rows.Scan(&m.TunnelID, &m.GatewayID, &m.GatewayName, &m.LocalSubnets); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate members")
		return
	}

	respondJSON(w, http.StatusOK, members)
}

// @Summary Add S2S tunnel member
// @Description Add a gateway as a member of a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Accept json
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param body body object true "Gateway ID and local subnets"
// @Success 201 "Created"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/members [post]
func (h *S2SHandler) addMember(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		GatewayID    string   `json:"gateway_id"`
		LocalSubnets []string `json:"local_subnets"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.GatewayID == "" {
		respondError(w, http.StatusBadRequest, "gateway_id is required")
		return
	}

	gatewayID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid gateway_id")
		return
	}

	subnets := req.LocalSubnets
	if subnets == nil {
		subnets = []string{}
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO s2s_tunnel_members (tunnel_id, gateway_id, local_subnets)
		 VALUES ($1, $2, $3::cidr[])
		 ON CONFLICT (tunnel_id, gateway_id) DO UPDATE SET local_subnets = $3::cidr[]`,
		tunnelID, gatewayID, subnets)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusBadRequest, "gateway or tunnel not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	h.log.Info("s2s member added", "tunnel_id", tunnelID, "gateway_id", gatewayID)
	if h.notifier != nil {
		h.notifier.NotifyS2SUpdate(req.GatewayID, tunnelID.String(), "add")
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "added", "tunnel_id": tunnelID.String(), "gateway_id": gatewayID.String()})
}

// @Summary Remove S2S tunnel member
// @Description Remove a gateway from a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param gatewayId path string true "Gateway ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/members/{gatewayId} [delete]
func (h *S2SHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	gatewayID, err := parseUUID(r, "gatewayId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM s2s_tunnel_members WHERE tunnel_id = $1 AND gateway_id = $2`,
		tunnelID, gatewayID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "member not found")
		return
	}

	if h.notifier != nil {
		h.notifier.NotifyS2SUpdate(gatewayID.String(), tunnelID.String(), "remove")
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Routes ---

type s2sRoute struct {
	ID          string `json:"id"`
	TunnelID    string `json:"tunnel_id"`
	Destination string `json:"destination"`
	ViaGateway  string `json:"via_gateway"`
	GatewayName string `json:"gateway_name"`
	Metric      int    `json:"metric"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
}

// @Summary List S2S tunnel routes
// @Description Returns all routes for a tunnel ordered by metric.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Success 200 {array} s2sRoute
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/routes [get]
func (h *S2SHandler) listRoutes(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT sr.id, sr.tunnel_id, sr.destination::text, sr.via_gateway, g.name, sr.metric, sr.is_active, sr.created_at
		 FROM s2s_routes sr
		 JOIN gateways g ON g.id = sr.via_gateway
		 WHERE sr.tunnel_id = $1
		 ORDER BY sr.metric`, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list routes")
		return
	}
	defer rows.Close()

	routes := make([]s2sRoute, 0)
	for rows.Next() {
		var rt s2sRoute
		if err := rows.Scan(&rt.ID, &rt.TunnelID, &rt.Destination, &rt.ViaGateway, &rt.GatewayName, &rt.Metric, &rt.IsActive, &rt.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan route")
			return
		}
		routes = append(routes, rt)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate routes")
		return
	}

	respondJSON(w, http.StatusOK, routes)
}

// @Summary Add S2S tunnel route
// @Description Add a route to a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Accept json
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param body body object true "Route data (destination CIDR, via_gateway, optional metric)"
// @Success 201 {object} s2sRoute
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/routes [post]
func (h *S2SHandler) addRoute(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Destination string `json:"destination"`
		ViaGateway  string `json:"via_gateway"`
		Metric      *int   `json:"metric,omitempty"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Destination == "" || req.ViaGateway == "" {
		respondError(w, http.StatusBadRequest, "destination and via_gateway are required")
		return
	}

	viaGW, err := uuid.Parse(req.ViaGateway)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid via_gateway")
		return
	}

	metric := 100
	if req.Metric != nil {
		metric = *req.Metric
	}

	var rt s2sRoute
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO s2s_routes (tunnel_id, destination, via_gateway, metric)
		 VALUES ($1, $2::cidr, $3, $4)
		 RETURNING id, tunnel_id, destination::text, via_gateway,
		           (SELECT name FROM gateways WHERE id = $3),
		           metric, is_active, created_at`,
		tunnelID, req.Destination, viaGW, metric,
	).Scan(&rt.ID, &rt.TunnelID, &rt.Destination, &rt.ViaGateway, &rt.GatewayName, &rt.Metric, &rt.IsActive, &rt.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23503" {
				respondError(w, http.StatusBadRequest, "tunnel or gateway not found")
				return
			}
			if pgErr.Code == "22P02" {
				respondError(w, http.StatusBadRequest, "invalid CIDR format for destination")
				return
			}
		}
		respondError(w, http.StatusInternalServerError, "failed to add route")
		return
	}

	h.log.Info("s2s route added", "tunnel_id", tunnelID, "destination", req.Destination, "via", viaGW)
	if h.notifier != nil {
		h.notifyTunnelMembers(r.Context(), tunnelID.String(), "route_add")
	}
	respondJSON(w, http.StatusCreated, rt)
}

// @Summary Remove S2S tunnel route
// @Description Remove a route from a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param routeId path string true "Route ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/routes/{routeId} [delete]
func (h *S2SHandler) removeRoute(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	routeID, err := parseUUID(r, "routeId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM s2s_routes WHERE id = $1 AND tunnel_id = $2`,
		routeID, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete route")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "route not found")
		return
	}

	if h.notifier != nil {
		h.notifyTunnelMembers(r.Context(), tunnelID.String(), "route_remove")
	}

	w.WriteHeader(http.StatusNoContent)
}

// notifyTunnelMembers sends an S2S update notification to all gateways
// that are members of the given tunnel.
func (h *S2SHandler) notifyTunnelMembers(ctx context.Context, tunnelID string, action string) {
	rows, err := h.pool.Query(ctx,
		`SELECT gateway_id::text FROM s2s_tunnel_members WHERE tunnel_id = $1`, tunnelID)
	if err != nil {
		h.log.Warn("failed to query tunnel members for notification", "tunnel_id", tunnelID, "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID string
		if err := rows.Scan(&gwID); err != nil {
			continue
		}
		h.notifier.NotifyS2SUpdate(gwID, tunnelID, action)
	}
}

// --- Config Generation ---

// @Summary Generate S2S tunnel config
// @Description Generate a WireGuard configuration file for a gateway in a tunnel.
// @Tags S2S Tunnels
// @Produce text/plain
// @Param id path string true "Tunnel ID (UUID)"
// @Param gatewayId path string true "Gateway ID (UUID)"
// @Success 200 {string} string "WireGuard config file"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/config/{gatewayId} [get]
func (h *S2SHandler) generateConfig(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	gatewayID, err := parseUUID(r, "gatewayId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify tunnel exists
	var tunnelName string
	err = h.pool.QueryRow(r.Context(),
		`SELECT name FROM s2s_tunnels WHERE id = $1`, tunnelID,
	).Scan(&tunnelName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "tunnel not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get tunnel")
		return
	}

	// Fetch all members with gateway info
	type memberInfo struct {
		GatewayID       string
		GatewayName     string
		WireguardPubkey string
		Endpoint        string
		LocalSubnets    []string
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.gateway_id, g.name, COALESCE(g.wireguard_pubkey, ''), COALESCE(g.endpoint, ''),
		        ARRAY(SELECT unnest(m.local_subnets)::text)
		 FROM s2s_tunnel_members m
		 JOIN gateways g ON g.id = m.gateway_id
		 WHERE m.tunnel_id = $1`, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query members")
		return
	}
	defer rows.Close()

	members := make([]memberInfo, 0)
	selfIdx := -1
	for rows.Next() {
		var m memberInfo
		if err := rows.Scan(&m.GatewayID, &m.GatewayName, &m.WireguardPubkey, &m.Endpoint, &m.LocalSubnets); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		if m.GatewayID == gatewayID.String() {
			selfIdx = len(members)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate members")
		return
	}

	if selfIdx < 0 {
		respondError(w, http.StatusNotFound, "gateway is not a member of this tunnel")
		return
	}
	self := &members[selfIdx]

	// Fetch active routes for this tunnel
	type routeInfo struct {
		Destination string
		ViaGateway  string
		IsActive    bool
	}

	routeRows, err := h.pool.Query(r.Context(),
		`SELECT destination::text, via_gateway, is_active
		 FROM s2s_routes
		 WHERE tunnel_id = $1 AND is_active = true
		 ORDER BY metric`, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query routes")
		return
	}
	defer routeRows.Close()

	// Map via_gateway -> list of route destinations
	routesByGateway := make(map[string][]string)
	for routeRows.Next() {
		var ri routeInfo
		if err := routeRows.Scan(&ri.Destination, &ri.ViaGateway, &ri.IsActive); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan route")
			return
		}
		routesByGateway[ri.ViaGateway] = append(routesByGateway[ri.ViaGateway], ri.Destination)
	}
	if err := routeRows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate routes")
		return
	}

	// Build WireGuard config
	var b strings.Builder

	// [Interface] section
	b.WriteString("# WireGuard S2S config for tunnel: " + tunnelName + "\n")
	b.WriteString("# Gateway: " + self.GatewayName + "\n")
	b.WriteString("# Generated: " + time.Now().UTC().Format(time.RFC3339) + "\n\n")
	b.WriteString("[Interface]\n")
	b.WriteString("PrivateKey = <PRIVATE_KEY>\n")
	if len(self.LocalSubnets) > 0 {
		b.WriteString("Address = " + strings.Join(self.LocalSubnets, ", ") + "\n")
	}

	// [Peer] sections for every other member
	for _, m := range members {
		if m.GatewayID == gatewayID.String() {
			continue
		}

		b.WriteString("\n# Peer: " + m.GatewayName + "\n")
		b.WriteString("[Peer]\n")
		if m.WireguardPubkey != "" {
			b.WriteString("PublicKey = " + m.WireguardPubkey + "\n")
		} else {
			b.WriteString("PublicKey = <MISSING_PUBKEY>\n")
		}
		if m.Endpoint != "" {
			b.WriteString("Endpoint = " + m.Endpoint + "\n")
		}

		// AllowedIPs = member's local_subnets + any routes via that gateway
		allowedIPs := make([]string, 0, len(m.LocalSubnets))
		allowedIPs = append(allowedIPs, m.LocalSubnets...)
		if extra, ok := routesByGateway[m.GatewayID]; ok {
			allowedIPs = append(allowedIPs, extra...)
		}
		if len(allowedIPs) > 0 {
			b.WriteString("AllowedIPs = " + strings.Join(allowedIPs, ", ") + "\n")
		}
		b.WriteString("PersistentKeepalive = 25\n")
	}

	filename := fmt.Sprintf("s2s-%s-%s.conf", tunnelName, self.GatewayName)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(b.String()))
}

// --- Allowed Domains ---

type s2sAllowedDomain struct {
	ID        string `json:"id"`
	TunnelID  string `json:"tunnel_id"`
	Domain    string `json:"domain"`
	CreatedAt string `json:"created_at"`
}

// @Summary List S2S tunnel allowed domains
// @Description Returns all allowed domains for a tunnel.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Success 200 {array} s2sAllowedDomain
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/domains [get]
func (h *S2SHandler) listDomains(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, tunnel_id, domain, created_at FROM s2s_allowed_domains WHERE tunnel_id = $1 ORDER BY created_at`, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list domains")
		return
	}
	defer rows.Close()

	domains := make([]s2sAllowedDomain, 0)
	for rows.Next() {
		var d s2sAllowedDomain
		if err := rows.Scan(&d.ID, &d.TunnelID, &d.Domain, &d.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan domain")
			return
		}
		domains = append(domains, d)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate domains")
		return
	}

	respondJSON(w, http.StatusOK, domains)
}

// @Summary Add S2S tunnel allowed domain
// @Description Add an allowed domain to a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Accept json
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param body body object true "Domain data"
// @Success 201 {object} s2sAllowedDomain
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/domains [post]
func (h *S2SHandler) addDomain(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Domain == "" {
		respondError(w, http.StatusBadRequest, "domain is required")
		return
	}

	var d s2sAllowedDomain
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO s2s_allowed_domains (tunnel_id, domain) VALUES ($1, $2)
		 RETURNING id, tunnel_id, domain, created_at`,
		tunnelID, req.Domain,
	).Scan(&d.ID, &d.TunnelID, &d.Domain, &d.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "domain already exists for this tunnel")
			return
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusNotFound, "tunnel not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to add domain")
		return
	}

	h.log.Info("s2s domain added", "tunnel_id", tunnelID, "domain", req.Domain)
	respondJSON(w, http.StatusCreated, d)
}

// @Summary Remove S2S tunnel allowed domain
// @Description Remove an allowed domain from a tunnel. Requires admin privileges.
// @Tags S2S Tunnels
// @Produce json
// @Param id path string true "Tunnel ID (UUID)"
// @Param domainId path string true "Domain ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /s2s-tunnels/{id}/domains/{domainId} [delete]
func (h *S2SHandler) removeDomain(w http.ResponseWriter, r *http.Request) {
	tunnelID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	domainID, err := parseUUID(r, "domainId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM s2s_allowed_domains WHERE id = $1 AND tunnel_id = $2`,
		domainID, tunnelID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "domain not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
