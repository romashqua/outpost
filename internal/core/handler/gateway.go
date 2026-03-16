package handler

import (
	"context"
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

type gatewayNetworkInfo struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
}

type gatewayResponse struct {
	ID              uuid.UUID          `json:"id"`
	NetworkID       *uuid.UUID         `json:"network_id"`
	Name            string             `json:"name"`
	PublicIP        *string            `json:"public_ip"`
	WireguardPubkey string             `json:"wireguard_pubkey"`
	Endpoint        string             `json:"endpoint"`
	IsActive        bool               `json:"is_active"`
	Priority        int                `json:"priority"`
	LastSeen        *time.Time         `json:"last_seen,omitempty"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	NetworkIDs      []uuid.UUID        `json:"network_ids"`
	Networks        []gatewayNetworkInfo `json:"networks"`
}

type gatewayCreateResponse struct {
	gatewayResponse
	Token string `json:"token"`
}

type createGatewayRequest struct {
	Name       string   `json:"name"`
	NetworkID  string   `json:"network_id"`
	NetworkIDs []string `json:"network_ids"`
	Endpoint   string   `json:"endpoint"`
	PublicIP   *string  `json:"public_ip,omitempty"`
	Priority   *int     `json:"priority,omitempty"`
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
	gatewayIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var g gatewayResponse
		if err := rows.Scan(&g.ID, &g.NetworkID, &g.Name, &g.PublicIP, &g.WireguardPubkey,
			&g.Endpoint, &g.IsActive, &g.Priority, &g.LastSeen, &g.CreatedAt, &g.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan gateway")
			return
		}
		g.NetworkIDs = []uuid.UUID{}
		g.Networks = []gatewayNetworkInfo{}
		gateways = append(gateways, g)
		gatewayIDs = append(gatewayIDs, g.ID)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate gateways")
		return
	}

	// Load networks for all gateways
	if len(gatewayIDs) > 0 {
		h.loadGatewayNetworks(r.Context(), gateways)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"gateways": gateways,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// loadGatewayNetworks populates NetworkIDs and Networks for each gateway from the junction table.
func (h *GatewayHandler) loadGatewayNetworks(ctx context.Context, gateways []gatewayResponse) {
	ids := make([]uuid.UUID, len(gateways))
	idxMap := make(map[uuid.UUID]int, len(gateways))
	for i, g := range gateways {
		ids[i] = g.ID
		idxMap[g.ID] = i
	}

	rows, err := h.pool.Query(ctx,
		`SELECT gn.gateway_id, n.id, n.name, n.address
		 FROM gateway_networks gn
		 JOIN networks n ON n.id = gn.network_id
		 WHERE gn.gateway_id = ANY($1)
		 ORDER BY n.name`, ids)
	if err != nil {
		h.log.Error("failed to load gateway networks", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var gwID, netID uuid.UUID
		var name, address string
		if err := rows.Scan(&gwID, &netID, &name, &address); err != nil {
			h.log.Error("failed to scan gateway network", "error", err)
			continue
		}
		if idx, ok := idxMap[gwID]; ok {
			gateways[idx].NetworkIDs = append(gateways[idx].NetworkIDs, netID)
			gateways[idx].Networks = append(gateways[idx].Networks, gatewayNetworkInfo{
				ID: netID, Name: name, Address: address,
			})
		}
	}
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
	if req.Endpoint == "" {
		respondError(w, http.StatusBadRequest, "endpoint is required")
		return
	}
	// Add default WireGuard port if not specified
	if !strings.Contains(req.Endpoint, ":") {
		req.Endpoint = req.Endpoint + ":51820"
	}

	// Collect network IDs: support both network_ids (new) and network_id (legacy).
	var networkIDs []uuid.UUID
	if len(req.NetworkIDs) > 0 {
		for _, nid := range req.NetworkIDs {
			parsed, err := uuid.Parse(nid)
			if err != nil {
				respondError(w, http.StatusBadRequest, "invalid network_id: "+nid)
				return
			}
			networkIDs = append(networkIDs, parsed)
		}
	} else if req.NetworkID != "" {
		parsed, err := uuid.Parse(req.NetworkID)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid network_id")
			return
		}
		networkIDs = append(networkIDs, parsed)
	}
	if len(networkIDs) == 0 {
		respondError(w, http.StatusBadRequest, "at least one network is required")
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

	// Use first network as legacy network_id for backward compatibility.
	primaryNetworkID := networkIDs[0]

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var g gatewayResponse
	err = tx.QueryRow(r.Context(),
		`INSERT INTO gateways (network_id, name, wireguard_pubkey, endpoint, token_hash, public_ip, priority)
		 VALUES ($1, $2, $3, $4, $5, $6::inet, $7)
		 RETURNING id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at`,
		primaryNetworkID, req.Name, pubKey, req.Endpoint, tokenHash, req.PublicIP, priority,
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

	// Insert into junction table.
	for _, nid := range networkIDs {
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO gateway_networks (gateway_id, network_id) VALUES ($1, $2)`,
			g.ID, nid); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to assign network to gateway")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	g.NetworkIDs = networkIDs
	g.Networks = []gatewayNetworkInfo{}
	h.loadGatewayNetworks(r.Context(), []gatewayResponse{g})
	if len(g.Networks) == 0 {
		// Fallback: populate from IDs we just inserted
		g.Networks = []gatewayNetworkInfo{}
	}

	resp := struct {
		gatewayResponse
		Token      string `json:"token"`
		PrivateKey string `json:"private_key"`
	}{
		gatewayResponse: g,
		Token:           token,
		PrivateKey:      privKey,
	}

	h.log.Info("gateway created", "id", g.ID, "name", g.Name, "networks", len(networkIDs), "endpoint", g.Endpoint)
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

	g.NetworkIDs = []uuid.UUID{}
	g.Networks = []gatewayNetworkInfo{}
	h.loadGatewayNetworks(r.Context(), []gatewayResponse{g})

	respondJSON(w, http.StatusOK, g)
}

type updateGatewayRequest struct {
	Name       *string  `json:"name,omitempty"`
	Endpoint   *string  `json:"endpoint,omitempty"`
	PublicIP   *string  `json:"public_ip,omitempty"`
	Priority   *int     `json:"priority,omitempty"`
	IsActive   *bool    `json:"is_active,omitempty"`
	NetworkIDs []string `json:"network_ids,omitempty"`
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

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var g gatewayResponse
	err = tx.QueryRow(r.Context(),
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

	// Update networks if provided.
	if len(req.NetworkIDs) > 0 {
		var networkIDs []uuid.UUID
		for _, nid := range req.NetworkIDs {
			parsed, parseErr := uuid.Parse(nid)
			if parseErr != nil {
				respondError(w, http.StatusBadRequest, "invalid network_id: "+nid)
				return
			}
			networkIDs = append(networkIDs, parsed)
		}

		// Replace all network assignments.
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM gateway_networks WHERE gateway_id = $1`, id); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update gateway networks")
			return
		}
		for _, nid := range networkIDs {
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO gateway_networks (gateway_id, network_id) VALUES ($1, $2)`,
				id, nid); err != nil {
				respondError(w, http.StatusInternalServerError, "failed to assign network to gateway")
				return
			}
		}

		// Update legacy network_id to first network.
		if _, err := tx.Exec(r.Context(),
			`UPDATE gateways SET network_id = $2 WHERE id = $1`,
			id, networkIDs[0]); err != nil {
			h.log.Error("failed to update legacy network_id", "error", err)
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	g.NetworkIDs = []uuid.UUID{}
	g.Networks = []gatewayNetworkInfo{}
	h.loadGatewayNetworks(r.Context(), []gatewayResponse{g})

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
