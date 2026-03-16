package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"github.com/romashqua/outpost/internal/wireguard"
)

// GatewayHandler provides endpoints for managing WireGuard gateways.
type GatewayHandler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewGatewayHandler creates a GatewayHandler backed by the given connection pool.
func NewGatewayHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *GatewayHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &GatewayHandler{pool: pool, log: l.With("handler", "gateway")}
}

// Routes returns a chi.Router with gateway management endpoints mounted.
func (h *GatewayHandler) Routes() chi.Router {
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

type gatewayResponse struct {
	ID              uuid.UUID  `json:"id"`
	NetworkID       uuid.UUID  `json:"network_id"`
	Name            string     `json:"name"`
	PublicIP        *string    `json:"public_ip"`
	WireguardPubkey string     `json:"wireguard_pubkey"`
	Endpoint        string     `json:"endpoint"`
	IsActive        bool       `json:"is_active"`
	Priority        int        `json:"priority"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type gatewayCreateResponse struct {
	gatewayResponse
	Token string `json:"token"`
}

type createGatewayRequest struct {
	Name      string  `json:"name"`
	NetworkID string  `json:"network_id"`
	Endpoint  string  `json:"endpoint"`
	PublicIP  *string `json:"public_ip,omitempty"`
	Priority  *int    `json:"priority,omitempty"`
}

// @Summary List gateways
// @Description Returns a paginated list of all gateways.
// @Tags Gateways
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(50)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /gateways [get]
func (h *GatewayHandler) list(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM gateways`).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count gateways")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at
		 FROM gateways
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query gateways")
		return
	}
	defer rows.Close()

	gateways := make([]gatewayResponse, 0)
	for rows.Next() {
		var g gatewayResponse
		if err := rows.Scan(&g.ID, &g.NetworkID, &g.Name, &g.PublicIP, &g.WireguardPubkey,
			&g.Endpoint, &g.IsActive, &g.Priority, &g.LastSeen, &g.CreatedAt, &g.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan gateway")
			return
		}
		gateways = append(gateways, g)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate gateways")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"gateways": gateways,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// @Summary Create gateway
// @Description Create a new WireGuard gateway. Returns a one-time authentication token. Requires admin privileges.
// @Tags Gateways
// @Accept json
// @Produce json
// @Param body body createGatewayRequest true "Gateway data"
// @Success 201 {object} gatewayCreateResponse
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /gateways [post]
func (h *GatewayHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createGatewayRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.NetworkID == "" {
		respondError(w, http.StatusBadRequest, "network_id is required")
		return
	}
	if req.Endpoint == "" {
		respondError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	// Add default WireGuard port if not specified
	if !strings.Contains(req.Endpoint, ":") {
		req.Endpoint = req.Endpoint + ":51820"
	}

	networkID, err := uuid.Parse(req.NetworkID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid network_id")
		return
	}

	// Generate WireGuard keypair for the gateway.
	privKey, err := wireguard.GeneratePrivateKey()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate keypair")
		return
	}
	pubKey, err := wireguard.PublicKey(privKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive public key")
		return
	}

	// Generate authentication token for the gateway.
	token, tokenHash, err := generateToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	var g gatewayResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO gateways (network_id, name, wireguard_pubkey, endpoint, token_hash, public_ip, priority)
		 VALUES ($1, $2, $3, $4, $5, $6::inet, $7)
		 RETURNING id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at`,
		networkID, req.Name, pubKey, req.Endpoint, tokenHash, req.PublicIP, priority,
	).Scan(&g.ID, &g.NetworkID, &g.Name, &g.PublicIP, &g.WireguardPubkey,
		&g.Endpoint, &g.IsActive, &g.Priority, &g.LastSeen, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "gateway already exists"
			if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "gateway with this name already exists"
			} else if strings.Contains(pgErr.ConstraintName, "endpoint") {
				msg = "gateway with this endpoint already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create gateway")
		return
	}

	// Return the plaintext token only on creation. It is never stored or
	// returned again. The private key is intentionally not stored in the
	// database; the gateway operator must save it from this response.
	resp := struct {
		gatewayResponse
		Token      string `json:"token"`
		PrivateKey string `json:"private_key"`
	}{
		gatewayResponse: g,
		Token:           token,
		PrivateKey:      privKey,
	}

	h.log.Info("gateway created", "id", g.ID, "name", g.Name, "network_id", g.NetworkID, "endpoint", g.Endpoint)
	respondJSON(w, http.StatusCreated, resp)
}

// @Summary Get gateway
// @Description Retrieve a gateway by ID.
// @Tags Gateways
// @Produce json
// @Param id path string true "Gateway ID (UUID)"
// @Success 200 {object} gatewayResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /gateways/{id} [get]
func (h *GatewayHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var g gatewayResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at
		 FROM gateways WHERE id = $1`, id,
	).Scan(&g.ID, &g.NetworkID, &g.Name, &g.PublicIP, &g.WireguardPubkey,
		&g.Endpoint, &g.IsActive, &g.Priority, &g.LastSeen, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "gateway not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch gateway")
		}
		return
	}

	respondJSON(w, http.StatusOK, g)
}

type updateGatewayRequest struct {
	Name     *string `json:"name,omitempty"`
	Endpoint *string `json:"endpoint,omitempty"`
	PublicIP *string `json:"public_ip,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

// @Summary Update gateway
// @Description Update an existing gateway. Requires admin privileges.
// @Tags Gateways
// @Accept json
// @Produce json
// @Param id path string true "Gateway ID (UUID)"
// @Param body body updateGatewayRequest true "Fields to update"
// @Success 200 {object} gatewayResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /gateways/{id} [put]
func (h *GatewayHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateGatewayRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var g gatewayResponse
	err = h.pool.QueryRow(r.Context(),
		`UPDATE gateways
		 SET name = COALESCE($2, name),
		     endpoint = COALESCE($3, endpoint),
		     public_ip = COALESCE($4::inet, public_ip),
		     priority = COALESCE($5, priority),
		     is_active = COALESCE($6, is_active),
		     updated_at = now()
		 WHERE id = $1
		 RETURNING id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at`,
		id, req.Name, req.Endpoint, req.PublicIP, req.Priority, req.IsActive,
	).Scan(&g.ID, &g.NetworkID, &g.Name, &g.PublicIP, &g.WireguardPubkey,
		&g.Endpoint, &g.IsActive, &g.Priority, &g.LastSeen, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "gateway not found")
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				respondError(w, http.StatusConflict, "gateway with this name or endpoint already exists")
			} else {
				respondError(w, http.StatusInternalServerError, "failed to update gateway")
			}
		}
		return
	}

	h.log.Info("gateway updated", "id", g.ID, "name", g.Name)
	respondJSON(w, http.StatusOK, g)
}

// @Summary Delete gateway
// @Description Delete a gateway by ID. Requires admin privileges.
// @Tags Gateways
// @Produce json
// @Param id path string true "Gateway ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /gateways/{id} [delete]
func (h *GatewayHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM gateways WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete gateway")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "gateway not found")
		return
	}

	h.log.Info("gateway deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// generateToken creates a cryptographically random 32-byte token, returning
// the hex-encoded plaintext and its SHA-256 hash for storage.
func generateToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(h[:])
	return plaintext, hash, nil
}
