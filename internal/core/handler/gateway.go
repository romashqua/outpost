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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

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
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Delete("/", h.delete)
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
	Name      string `json:"name"`
	NetworkID string `json:"network_id"`
	Endpoint  string `json:"endpoint"`
}

func (h *GatewayHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at
		 FROM gateways
		 ORDER BY created_at DESC`)
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

	respondJSON(w, http.StatusOK, gateways)
}

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

	var g gatewayResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO gateways (network_id, name, wireguard_pubkey, endpoint, token_hash)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, network_id, name, public_ip::text, wireguard_pubkey, endpoint, is_active, priority, last_seen, created_at, updated_at`,
		networkID, req.Name, pubKey, req.Endpoint, tokenHash,
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
		respondError(w, http.StatusNotFound, "gateway not found")
		return
	}

	respondJSON(w, http.StatusOK, g)
}

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
