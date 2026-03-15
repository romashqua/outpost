package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
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

// NetworkHandler provides CRUD endpoints for network management.
type NetworkHandler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewNetworkHandler creates a NetworkHandler backed by the given connection pool.
func NewNetworkHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *NetworkHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &NetworkHandler{pool: pool, log: l.With("handler", "network")}
}

// Routes returns a chi.Router with network CRUD endpoints mounted.
func (h *NetworkHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.With(auth.RequireAdmin).Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.With(auth.RequireAdmin).Put("/", h.update)
		r.With(auth.RequireAdmin).Delete("/", h.delete)
	})
	return r
}

type networkResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	DNS       []string  `json:"dns"`
	Port      int       `json:"port"`
	Keepalive int       `json:"keepalive"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type createNetworkRequest struct {
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	DNS       []string `json:"dns"`
	Port      int      `json:"port"`
	Keepalive int      `json:"keepalive"`
}

type updateNetworkRequest struct {
	Name      *string  `json:"name,omitempty"`
	Address   *string  `json:"address,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	Port      *int     `json:"port,omitempty"`
	Keepalive *int     `json:"keepalive,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

func (h *NetworkHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, address::text, dns, port, keepalive, is_active, created_at, updated_at
		 FROM networks
		 ORDER BY created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query networks")
		return
	}
	defer rows.Close()

	networks := make([]networkResponse, 0)
	for rows.Next() {
		var n networkResponse
		if err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.DNS, &n.Port,
			&n.Keepalive, &n.IsActive, &n.CreatedAt, &n.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan network")
			return
		}
		networks = append(networks, n)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate networks")
		return
	}

	respondJSON(w, http.StatusOK, networks)
}

func (h *NetworkHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createNetworkRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Address == "" {
		respondError(w, http.StatusBadRequest, "address (CIDR) is required")
		return
	}

	// Validate CIDR format and ensure it's a network address (no host bits set).
	ip, ipNet, err := net.ParseCIDR(req.Address)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid CIDR format: %s — expected format like 10.0.0.0/24", req.Address))
		return
	}
	if !ip.Equal(ipNet.IP) {
		respondError(w, http.StatusBadRequest, fmt.Sprintf(
			"invalid network address: %s has host bits set — did you mean %s?",
			req.Address, ipNet.String()))
		return
	}

	if req.Port == 0 {
		req.Port = 51820
	}
	if req.Port < 1 || req.Port > 65535 {
		respondError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.Keepalive == 0 {
		req.Keepalive = 25
	}
	if req.Keepalive < 0 || req.Keepalive > 3600 {
		respondError(w, http.StatusBadRequest, "keepalive must be between 0 and 3600")
		return
	}
	if req.DNS == nil {
		req.DNS = []string{}
	}

	var n networkResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO networks (name, address, dns, port, keepalive)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, address::text, dns, port, keepalive, is_active, created_at, updated_at`,
		req.Name, req.Address, req.DNS, req.Port, req.Keepalive,
	).Scan(&n.ID, &n.Name, &n.Address, &n.DNS, &n.Port,
		&n.Keepalive, &n.IsActive, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "network already exists"
			if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "network with this name already exists"
			} else if strings.Contains(pgErr.ConstraintName, "address") {
				msg = "network with this address already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		h.log.Error("failed to create network", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create network")
		return
	}

	h.log.Info("network created", "id", n.ID, "name", n.Name, "address", n.Address)
	respondJSON(w, http.StatusCreated, n)
}

func (h *NetworkHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var n networkResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, address::text, dns, port, keepalive, is_active, created_at, updated_at
		 FROM networks WHERE id = $1`, id,
	).Scan(&n.ID, &n.Name, &n.Address, &n.DNS, &n.Port,
		&n.Keepalive, &n.IsActive, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "network not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch network")
		}
		return
	}

	respondJSON(w, http.StatusOK, n)
}

func (h *NetworkHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateNetworkRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate CIDR format if address is being updated.
	if req.Address != nil && *req.Address != "" {
		ip, ipNet, err := net.ParseCIDR(*req.Address)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid CIDR format: %s", *req.Address))
			return
		}
		if !ip.Equal(ipNet.IP) {
			respondError(w, http.StatusBadRequest, fmt.Sprintf(
				"invalid network address: %s has host bits set — did you mean %s?",
				*req.Address, ipNet.String()))
			return
		}
	}

	// Validate port and keepalive if provided.
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		respondError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.Keepalive != nil && (*req.Keepalive < 0 || *req.Keepalive > 3600) {
		respondError(w, http.StatusBadRequest, "keepalive must be between 0 and 3600")
		return
	}

	var n networkResponse
	err = h.pool.QueryRow(r.Context(),
		`UPDATE networks SET
			name      = COALESCE($2, name),
			address   = COALESCE($3, address),
			dns       = COALESCE($4, dns),
			port      = COALESCE($5, port),
			keepalive = COALESCE($6, keepalive),
			is_active = COALESCE($7, is_active),
			updated_at = now()
		 WHERE id = $1
		 RETURNING id, name, address::text, dns, port, keepalive, is_active, created_at, updated_at`,
		id, req.Name, req.Address, req.DNS, req.Port, req.Keepalive, req.IsActive,
	).Scan(&n.ID, &n.Name, &n.Address, &n.DNS, &n.Port,
		&n.Keepalive, &n.IsActive, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "network not found")
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				respondError(w, http.StatusConflict, "network with this name already exists")
			} else {
				respondError(w, http.StatusInternalServerError, "failed to update network")
			}
		}
		return
	}

	respondJSON(w, http.StatusOK, n)
}

func (h *NetworkHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM networks WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete network")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "network not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
