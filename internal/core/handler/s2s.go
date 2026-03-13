package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type S2SHandler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewS2SHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *S2SHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &S2SHandler{pool: pool, log: l.With("handler", "s2s")}
}

func (h *S2SHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Delete("/", h.delete)
		r.Get("/members", h.listMembers)
		r.Post("/members", h.addMember)
		r.Delete("/members/{gatewayId}", h.removeMember)
		r.Get("/routes", h.listRoutes)
		r.Post("/routes", h.addRoute)
		r.Delete("/routes/{routeId}", h.removeRoute)
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
		respondError(w, http.StatusNotFound, "tunnel not found")
		return
	}

	respondJSON(w, http.StatusOK, t)
}

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

	respondJSON(w, http.StatusOK, members)
}

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
	w.WriteHeader(http.StatusCreated)
}

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

	respondJSON(w, http.StatusOK, routes)
}

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
		 RETURNING id, tunnel_id, destination::text, via_gateway, '', metric, is_active, created_at::text`,
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
	respondJSON(w, http.StatusCreated, rt)
}

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

	w.WriteHeader(http.StatusNoContent)
}
