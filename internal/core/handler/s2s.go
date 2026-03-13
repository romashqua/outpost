package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
	r.Get("/{id}", h.get)
	r.Delete("/{id}", h.delete)
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
