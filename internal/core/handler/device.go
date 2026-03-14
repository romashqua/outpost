package handler

import (
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
	"github.com/romashqua/outpost/internal/wireguard"
)

// DeviceHandler provides endpoints for managing WireGuard devices (peers).
type DeviceHandler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewDeviceHandler creates a DeviceHandler backed by the given connection pool.
func NewDeviceHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *DeviceHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &DeviceHandler{pool: pool, log: l.With("handler", "device")}
}

// Routes returns a chi.Router with device management endpoints mounted.
func (h *DeviceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/my", h.listMy)
	r.Post("/", h.create)
	r.Post("/enroll", h.enroll)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Get("/config", h.downloadConfig)
		r.Delete("/", h.delete)
		r.Post("/approve", h.approve)
		r.Post("/revoke", h.revoke)
	})
	return r
}

type deviceResponse struct {
	ID              uuid.UUID  `json:"id"`
	UserID          uuid.UUID  `json:"user_id"`
	Name            string     `json:"name"`
	WireguardPubkey string     `json:"wireguard_pubkey"`
	AssignedIP      string     `json:"assigned_ip"`
	IsApproved      bool       `json:"is_approved"`
	LastHandshake   *time.Time `json:"last_handshake,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type createDeviceRequest struct {
	Name            string `json:"name"`
	WireguardPubkey string `json:"wireguard_pubkey"`
	UserID          string `json:"user_id"`
}

type enrollRequest struct {
	Name            string `json:"name"`
	WireguardPubkey string `json:"wireguard_pubkey"`
}

type enrollResponse struct {
	DeviceID           uuid.UUID `json:"device_id"`
	Address            string    `json:"address"`
	DNS                []string  `json:"dns"`
	Endpoint           string    `json:"endpoint"`
	ServerPublicKey    string    `json:"server_public_key"`
	AllowedIPs         []string  `json:"allowed_ips"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
}

type configResponse struct {
	Config     string `json:"config"`
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}

func (h *DeviceHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at
		 FROM devices
		 ORDER BY created_at DESC`)
	if err != nil {
		h.log.Error("failed to query devices", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query devices")
		return
	}
	defer rows.Close()

	devices := make([]deviceResponse, 0)
	for rows.Next() {
		var d deviceResponse
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
			&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan device")
			return
		}
		devices = append(devices, d)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate devices")
		return
	}

	respondJSON(w, http.StatusOK, devices)
}

func (h *DeviceHandler) listMy(w http.ResponseWriter, r *http.Request) {
	// In production, the user ID comes from the authenticated session/JWT.
	// For now, require it as a query parameter.
	rawUserID := r.URL.Query().Get("user_id")
	if rawUserID == "" {
		respondError(w, http.StatusBadRequest, "user_id query parameter is required")
		return
	}
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at
		 FROM devices
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query devices")
		return
	}
	defer rows.Close()

	devices := make([]deviceResponse, 0)
	for rows.Next() {
		var d deviceResponse
		if err := rows.Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
			&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan device")
			return
		}
		devices = append(devices, d)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate devices")
		return
	}

	respondJSON(w, http.StatusOK, devices)
}

func (h *DeviceHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.WireguardPubkey == "" || req.WireguardPubkey == "auto-generated" {
		privKey, err := wireguard.GeneratePrivateKey()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate key pair")
			return
		}
		pubKey, err := wireguard.PublicKey(privKey)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to derive public key")
			return
		}
		req.WireguardPubkey = pubKey
	}

	// Accept user_id from the JSON body. Fall back to query parameter for
	// backward compatibility.
	rawUserID := req.UserID
	if rawUserID == "" {
		rawUserID = r.URL.Query().Get("user_id")
	}
	if rawUserID == "" {
		respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	// Assign the next available IP from the first active network using a
	// transaction with row locking to prevent race conditions.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var assignedIP string
	err = tx.QueryRow(r.Context(),
		`SELECT host(network(address) + (SELECT COUNT(*) + 2 FROM devices))
		 FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1
		 FOR UPDATE`,
	).Scan(&assignedIP)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address")
		return
	}

	var d deviceResponse
	err = tx.QueryRow(r.Context(),
		`INSERT INTO devices (user_id, name, wireguard_pubkey, assigned_ip)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at`,
		userID, req.Name, req.WireguardPubkey, assignedIP,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
		&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		h.log.Error("failed to create device", "error", err, "name", req.Name, "user_id", userID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "device already exists"
			if strings.Contains(pgErr.ConstraintName, "pubkey") {
				msg = "device with this public key already exists"
			} else if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "device with this name already exists"
			} else if strings.Contains(pgErr.ConstraintName, "assigned_ip") {
				msg = "IP address already in use"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create device")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit device creation")
		return
	}

	h.log.Info("device created", "id", d.ID, "name", d.Name, "user_id", d.UserID, "ip", d.AssignedIP)
	respondJSON(w, http.StatusCreated, d)
}

func (h *DeviceHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var d deviceResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at
		 FROM devices WHERE id = $1`, id,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
		&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		h.log.Error("failed to get device", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to get device")
		return
	}

	respondJSON(w, http.StatusOK, d)
}

func (h *DeviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		h.log.Error("failed to delete device", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to delete device")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	h.log.Info("device deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *DeviceHandler) approve(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE devices SET is_approved = true, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to approve device")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (h *DeviceHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE devices SET is_approved = false, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *DeviceHandler) enroll(w http.ResponseWriter, r *http.Request) {
	var req enrollRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.WireguardPubkey == "" {
		respondError(w, http.StatusBadRequest, "wireguard_pubkey is required")
		return
	}

	// Use a transaction with row locking to prevent IP allocation race conditions.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	// Auto-assign IP from the first active network and read its DNS servers.
	var assignedIP string
	var maskLen int
	var networkDNS []string
	err = tx.QueryRow(r.Context(),
		`SELECT host(network(address) + (SELECT COUNT(*) + 2 FROM devices)),
		        masklen(address),
		        dns
		 FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1
		 FOR UPDATE`,
	).Scan(&assignedIP, &maskLen, &networkDNS)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address: no active network found")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// Get the gateway endpoint and public key from the first active gateway.
	var gatewayEndpoint, gatewayPubkey string
	err = tx.QueryRow(r.Context(),
		`SELECT g.endpoint, g.wireguard_pubkey
		 FROM gateways g
		 JOIN networks n ON n.id = g.network_id
		 WHERE n.is_active = true AND g.is_active = true
		 ORDER BY g.priority DESC, g.created_at LIMIT 1`,
	).Scan(&gatewayEndpoint, &gatewayPubkey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusUnprocessableEntity, "no active gateway available — create and activate a gateway first")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to read gateway configuration")
		return
	}

	// Use a default user for enrollment (the first admin user). In production
	// this would come from the authenticated session.
	var userID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`SELECT id FROM users WHERE is_admin = true ORDER BY created_at LIMIT 1`,
	).Scan(&userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to find admin user for enrollment")
		return
	}

	// Create the device.
	var deviceID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO devices (user_id, name, wireguard_pubkey, assigned_ip, is_approved)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING id`,
		userID, req.Name, req.WireguardPubkey, assignedIP,
	).Scan(&deviceID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "device already exists"
			if strings.Contains(pgErr.ConstraintName, "pubkey") {
				msg = "device with this public key already exists"
			} else if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "device with this name already exists"
			} else if strings.Contains(pgErr.ConstraintName, "assigned_ip") {
				msg = "IP address already in use"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create device")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit enrollment")
		return
	}

	clientAddress := fmt.Sprintf("%s/%d", assignedIP, maskLen)

	resp := enrollResponse{
		DeviceID:            deviceID,
		Address:             clientAddress,
		DNS:                 networkDNS,
		Endpoint:            gatewayEndpoint,
		ServerPublicKey:     gatewayPubkey,
		AllowedIPs:          []string{"0.0.0.0/0"},
		PersistentKeepalive: 25,
	}

	h.log.Info("device enrolled", "device_id", deviceID, "name", req.Name, "address", clientAddress, "gateway", gatewayEndpoint)
	respondJSON(w, http.StatusCreated, resp)
}

func (h *DeviceHandler) downloadConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Fetch device info.
	var assignedIP string
	var deviceName string
	err = h.pool.QueryRow(r.Context(),
		`SELECT name, host(assigned_ip) FROM devices WHERE id = $1`, id,
	).Scan(&deviceName, &assignedIP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch device")
		}
		return
	}

	// Get the mask length and DNS from the active network.
	var maskLen int
	var networkDNS []string
	err = h.pool.QueryRow(r.Context(),
		`SELECT masklen(address), dns FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&maskLen, &networkDNS)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read network mask")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// Get the gateway endpoint and public key.
	var gatewayEndpoint, gatewayPubkey string
	err = h.pool.QueryRow(r.Context(),
		`SELECT g.endpoint, g.wireguard_pubkey
		 FROM gateways g
		 JOIN networks n ON n.id = g.network_id
		 WHERE n.is_active = true AND g.is_active = true
		 ORDER BY g.priority DESC, g.created_at LIMIT 1`,
	).Scan(&gatewayEndpoint, &gatewayPubkey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusUnprocessableEntity, "no active gateway available — create and activate a gateway first")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to read gateway configuration")
		return
	}

	// Generate a new key pair for the client. Since we only store the public
	// key, we generate a fresh pair and return the private key in the config.
	privateKey, err := wireguard.GeneratePrivateKey()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate WireGuard key pair")
		return
	}
	publicKey, err := wireguard.PublicKey(privateKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to derive public key")
		return
	}

	// Update the device's stored public key to match the newly generated pair.
	_, err = h.pool.Exec(r.Context(),
		`UPDATE devices SET wireguard_pubkey = $1, updated_at = now() WHERE id = $2`,
		publicKey, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update device public key")
		return
	}

	clientAddress := fmt.Sprintf("%s/%d", assignedIP, maskLen)

	cfg := wireguard.InterfaceConfig{
		PrivateKey: privateKey,
		Address:    clientAddress,
		DNS:        networkDNS,
		Peers: []wireguard.PeerConfig{
			{
				PublicKey:           gatewayPubkey,
				AllowedIPs:          []string{"0.0.0.0/0"},
				Endpoint:            gatewayEndpoint,
				PersistentKeepalive: 25,
			},
		},
	}

	configText := wireguard.RenderConfig(cfg)

	resp := configResponse{
		Config:     configText,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}

	respondJSON(w, http.StatusOK, resp)
}
