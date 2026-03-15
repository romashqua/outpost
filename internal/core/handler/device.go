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
	"github.com/romashqua/outpost/internal/wireguard"
)

// PeerNotifier is called when a device peer changes so gateways can be updated.
type PeerNotifier interface {
	NotifyPeerAdd(pubkey string, allowedIPs []string)
	NotifyPeerRemove(pubkey string)
}

// DeviceMailer sends device-related emails (satisfied by mail.Mailer).
type DeviceMailer interface {
	SendDeviceConfig(ctx context.Context, to, deviceName, configText string) error
}

// DeviceHandler provides endpoints for managing WireGuard devices (peers).
type DeviceHandler struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	notifier PeerNotifier
	mailer   DeviceMailer
}

// NewDeviceHandler creates a DeviceHandler backed by the given connection pool.
func NewDeviceHandler(pool *pgxpool.Pool, logger ...*slog.Logger) *DeviceHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &DeviceHandler{pool: pool, log: l.With("handler", "device")}
}

// WithNotifier sets the peer notifier for broadcasting changes to gateways.
func (h *DeviceHandler) WithNotifier(n PeerNotifier) *DeviceHandler {
	h.notifier = n
	return h
}

// WithMailer sets the mailer for sending config emails.
func (h *DeviceHandler) WithMailer(m DeviceMailer) *DeviceHandler {
	h.mailer = m
	return h
}

// Routes returns a chi.Router with device management endpoints mounted.
func (h *DeviceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.With(auth.RequireAdmin).Get("/", h.list)
	r.Get("/my", h.listMy)
	r.Post("/", h.create)
	r.Post("/enroll", h.enroll)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Get("/config", h.downloadConfig)
		r.Post("/send-config", h.sendConfig)
		r.Delete("/", h.delete)
		r.With(auth.RequireAdmin).Post("/approve", h.approve)
		r.With(auth.RequireAdmin).Post("/revoke", h.revoke)
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

// @Summary List all devices
// @Description Returns a paginated list of all devices. Requires admin privileges.
// @Tags Devices
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(50)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices [get]
func (h *DeviceHandler) list(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM devices`).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count devices")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at
		 FROM devices
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, perPage, offset)
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

	respondJSON(w, http.StatusOK, map[string]any{
		"devices":  devices,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// @Summary List my devices
// @Description Returns a paginated list of devices belonging to the authenticated user.
// @Tags Devices
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(50)
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/my [get]
func (h *DeviceHandler) listMy(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid user ID in token")
		return
	}

	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM devices WHERE user_id = $1`, userID).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count devices")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at
		 FROM devices
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, userID, perPage, offset)
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

	respondJSON(w, http.StatusOK, map[string]any{
		"devices":  devices,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// @Summary Create device
// @Description Create a new WireGuard device (peer). Auto-generates keys if not provided.
// @Tags Devices
// @Accept json
// @Produce json
// @Param body body createDeviceRequest true "Device data"
// @Success 201 {object} deviceResponse
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices [post]
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

	var generatedPrivKey string
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
		generatedPrivKey = privKey
	} else if !validWireGuardKey(req.WireguardPubkey) {
		respondError(w, http.StatusBadRequest, "invalid WireGuard public key: must be 44-character base64 (32 bytes)")
		return
	}

	// Non-admins can only create devices for themselves.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	rawUserID := req.UserID
	if rawUserID == "" {
		rawUserID = r.URL.Query().Get("user_id")
	}
	if rawUserID == "" {
		// Default to authenticated user.
		rawUserID = claims.UserID
	}

	if !claims.IsAdmin && rawUserID != claims.UserID {
		respondError(w, http.StatusForbidden, "you can only create devices for yourself")
		return
	}
	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	// Atomically allocate the next available IP and insert the device.
	// Uses a CTE that finds the first unused IP in the network range,
	// with a retry loop to handle concurrent inserts racing for the same IP.
	const maxRetries = 5
	var d deviceResponse
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = h.pool.QueryRow(r.Context(),
			`WITH net AS (
				SELECT id, address FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1
			),
			candidate AS (
				SELECT host(network(net.address) + s.off) AS ip, net.id AS net_id
				FROM net, generate_series(2, (1 << (32 - masklen(net.address))) - 2) AS s(off)
				WHERE NOT EXISTS (
					SELECT 1 FROM devices WHERE assigned_ip = (network(net.address) + s.off)::inet
				)
				LIMIT 1
			)
			INSERT INTO devices (user_id, name, wireguard_pubkey, wireguard_privkey, assigned_ip, network_id)
			SELECT $1, $2, $3, $4, candidate.ip::inet, candidate.net_id
			FROM candidate
			RETURNING id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at`,
			userID, req.Name, req.WireguardPubkey, ptrOrNil(generatedPrivKey),
		).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
			&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt)
		if err == nil {
			break
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "assigned_ip") {
				// IP collision from concurrent insert — retry with next available.
				h.log.Warn("IP allocation collision, retrying", "attempt", attempt+1)
				continue
			}
			msg := "device already exists"
			if strings.Contains(pgErr.ConstraintName, "pubkey") {
				msg = "device with this public key already exists"
			} else if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "device with this name already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusInternalServerError, "no available IP addresses in the network")
			return
		}
		h.log.Error("failed to create device", "error", err, "name", req.Name, "user_id", userID)
		respondError(w, http.StatusInternalServerError, "failed to create device")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address after retries")
		return
	}

	h.log.Info("device created", "id", d.ID, "name", d.Name, "user_id", d.UserID, "ip", d.AssignedIP)
	respondJSON(w, http.StatusCreated, d)
}

// @Summary Get device
// @Description Retrieve a device by ID. Non-admins can only view their own devices.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 200 {object} deviceResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id} [get]
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

	// Non-admins can only view their own devices.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !claims.IsAdmin && claims.UserID != d.UserID.String() {
		respondError(w, http.StatusForbidden, "you can only view your own devices")
		return
	}

	respondJSON(w, http.StatusOK, d)
}

// @Summary Delete device
// @Description Delete a device by ID. Non-admins can only delete their own devices.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id} [delete]
func (h *DeviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify ownership: non-admins can only delete their own devices.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Fetch device owner and pubkey before deleting.
	var ownerID, pubkey string
	err = h.pool.QueryRow(r.Context(),
		`SELECT user_id::text, wireguard_pubkey FROM devices WHERE id = $1`, id,
	).Scan(&ownerID, &pubkey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch device")
		return
	}

	if !claims.IsAdmin && claims.UserID != ownerID {
		respondError(w, http.StatusForbidden, "you can only delete your own devices")
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

	if h.notifier != nil && pubkey != "" {
		h.notifier.NotifyPeerRemove(pubkey)
	}

	h.log.Info("device deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// @Summary Approve device
// @Description Approve a pending device and notify gateways. Requires admin privileges.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id}/approve [post]
func (h *DeviceHandler) approve(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var pubkey, assignedIP string
	err = h.pool.QueryRow(r.Context(),
		`UPDATE devices SET is_approved = true, updated_at = now()
		 WHERE id = $1
		 RETURNING wireguard_pubkey, host(assigned_ip) || '/32'`,
		id,
	).Scan(&pubkey, &assignedIP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to approve device")
		return
	}

	if h.notifier != nil && pubkey != "" {
		h.notifier.NotifyPeerAdd(pubkey, []string{assignedIP})
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

// @Summary Revoke device
// @Description Revoke an approved device and remove its peer from gateways. Requires admin privileges.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id}/revoke [post]
func (h *DeviceHandler) revoke(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var pubkey string
	err = h.pool.QueryRow(r.Context(),
		`UPDATE devices SET is_approved = false, updated_at = now()
		 WHERE id = $1
		 RETURNING wireguard_pubkey`,
		id,
	).Scan(&pubkey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}

	if h.notifier != nil && pubkey != "" {
		h.notifier.NotifyPeerRemove(pubkey)
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// @Summary Enroll device
// @Description Self-service device enrollment. Auto-approves and returns connection parameters.
// @Tags Devices
// @Accept json
// @Produce json
// @Param body body enrollRequest true "Enrollment data"
// @Success 201 {object} enrollResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/enroll [post]
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
	if !validWireGuardKey(req.WireguardPubkey) {
		respondError(w, http.StatusBadRequest, "invalid WireGuard public key: must be 44-character base64 (32 bytes)")
		return
	}

	// Read network configuration (DNS, mask) and gateway info.
	var err error
	var maskLen int
	var networkDNS []string
	err = h.pool.QueryRow(r.Context(),
		`SELECT masklen(address), dns
		 FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&maskLen, &networkDNS)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address: no active network found")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// Get the gateway endpoint and public key from the first active gateway.
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

	// Use the authenticated user for enrollment.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	userID, parseErr := uuid.Parse(claims.UserID)
	if parseErr != nil {
		respondError(w, http.StatusInternalServerError, "invalid user ID in token")
		return
	}

	// Atomically allocate the next available IP and insert the device.
	// Uses a CTE that finds the first unused IP in the network range,
	// with a retry loop to handle concurrent inserts racing for the same IP.
	const maxEnrollRetries = 5
	var deviceID uuid.UUID
	var assignedIP string
	for attempt := 0; attempt < maxEnrollRetries; attempt++ {
		err = h.pool.QueryRow(r.Context(),
			`WITH net AS (
				SELECT id, address FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1
			),
			candidate AS (
				SELECT host(network(net.address) + s.off) AS ip, net.id AS net_id
				FROM net, generate_series(2, (1 << (32 - masklen(net.address))) - 2) AS s(off)
				WHERE NOT EXISTS (
					SELECT 1 FROM devices WHERE assigned_ip = (network(net.address) + s.off)::inet
				)
				LIMIT 1
			)
			INSERT INTO devices (user_id, name, wireguard_pubkey, assigned_ip, is_approved, network_id)
			SELECT $1, $2, $3, candidate.ip::inet, true, candidate.net_id
			FROM candidate
			RETURNING id, host(assigned_ip)`,
			userID, req.Name, req.WireguardPubkey,
		).Scan(&deviceID, &assignedIP)
		if err == nil {
			break
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "assigned_ip") {
				// IP collision from concurrent insert — retry with next available.
				h.log.Warn("IP allocation collision during enrollment, retrying", "attempt", attempt+1)
				continue
			}
			msg := "device already exists"
			if strings.Contains(pgErr.ConstraintName, "pubkey") {
				msg = "device with this public key already exists"
			} else if strings.Contains(pgErr.ConstraintName, "name") {
				msg = "device with this name already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusInternalServerError, "no available IP addresses in the network")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create device")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address after retries")
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

	// Notify gateways about the new peer (enroll auto-approves).
	if h.notifier != nil && req.WireguardPubkey != "" {
		h.notifier.NotifyPeerAdd(req.WireguardPubkey, []string{assignedIP + "/32"})
	}

	h.log.Info("device enrolled", "device_id", deviceID, "name", req.Name, "address", clientAddress, "gateway", gatewayEndpoint)
	respondJSON(w, http.StatusCreated, resp)
}

// @Summary Download device config
// @Description Generate and return a WireGuard configuration file for the device.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 200 {object} configResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id}/config [get]
func (h *DeviceHandler) downloadConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify ownership: non-admins can only download config for their own devices.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Fetch device info including owner.
	var assignedIP string
	var deviceName string
	var ownerID string
	err = h.pool.QueryRow(r.Context(),
		`SELECT name, host(assigned_ip), user_id::text FROM devices WHERE id = $1`, id,
	).Scan(&deviceName, &assignedIP, &ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch device")
		}
		return
	}

	if !claims.IsAdmin && claims.UserID != ownerID {
		respondError(w, http.StatusForbidden, "you can only download config for your own devices")
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

	// Look up the device's stored private key (set at creation time).
	// If the private key was not stored (legacy device), generate a new pair.
	var storedPrivKey *string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT wireguard_privkey FROM devices WHERE id = $1`, id,
	).Scan(&storedPrivKey)

	var privateKey, publicKey string
	if storedPrivKey != nil && *storedPrivKey != "" {
		privateKey = *storedPrivKey
		publicKey, err = wireguard.PublicKey(privateKey)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to derive public key")
			return
		}
	} else {
		// Fallback: generate a new key pair and update the device record.
		privateKey, err = wireguard.GeneratePrivateKey()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate WireGuard key pair")
			return
		}
		publicKey, err = wireguard.PublicKey(privateKey)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to derive public key")
			return
		}
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE devices SET wireguard_pubkey = $1, wireguard_privkey = $2, updated_at = now() WHERE id = $3`,
			publicKey, privateKey, id); err != nil {
			h.log.Error("failed to update device keys", "error", err, "device_id", id)
			respondError(w, http.StatusInternalServerError, "failed to update device keys")
			return
		}
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

// @Summary Send device config via email
// @Description Email the WireGuard configuration to the device owner.
// @Tags Devices
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 422 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id}/send-config [post]
func (h *DeviceHandler) sendConfig(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		respondError(w, http.StatusUnprocessableEntity, "email is not configured on this instance")
		return
	}

	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify ownership.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Fetch device owner, name, and email.
	var ownerID, deviceName, userEmail string
	err = h.pool.QueryRow(r.Context(),
		`SELECT d.user_id::text, d.name, u.email
		 FROM devices d JOIN users u ON u.id = d.user_id
		 WHERE d.id = $1`, id,
	).Scan(&ownerID, &deviceName, &userEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch device")
		return
	}

	if !claims.IsAdmin && claims.UserID != ownerID {
		respondError(w, http.StatusForbidden, "you can only send config for your own devices")
		return
	}

	// Build the config (same logic as downloadConfig).
	var assignedIP string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT host(assigned_ip) FROM devices WHERE id = $1`, id,
	).Scan(&assignedIP)

	var maskLen int
	var networkDNS []string
	err = h.pool.QueryRow(r.Context(),
		`SELECT masklen(address), dns FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&maskLen, &networkDNS)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read network configuration")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	var gatewayEndpoint, gatewayPubkey string
	err = h.pool.QueryRow(r.Context(),
		`SELECT g.endpoint, g.wireguard_pubkey
		 FROM gateways g JOIN networks n ON n.id = g.network_id
		 WHERE n.is_active = true AND g.is_active = true
		 ORDER BY g.priority DESC, g.created_at LIMIT 1`,
	).Scan(&gatewayEndpoint, &gatewayPubkey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read gateway configuration")
		return
	}

	var storedPrivKey *string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT wireguard_privkey FROM devices WHERE id = $1`, id,
	).Scan(&storedPrivKey)

	var privateKey string
	if storedPrivKey != nil && *storedPrivKey != "" {
		privateKey = *storedPrivKey
	} else {
		privateKey, err = wireguard.GeneratePrivateKey()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to generate key pair")
			return
		}
		pubKey, pubErr := wireguard.PublicKey(privateKey)
		if pubErr != nil {
			respondError(w, http.StatusInternalServerError, "failed to derive public key")
			return
		}
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE devices SET wireguard_pubkey = $1, wireguard_privkey = $2, updated_at = now() WHERE id = $3`,
			pubKey, privateKey, id); err != nil {
			h.log.Error("failed to update device keys", "error", err, "device_id", id)
			respondError(w, http.StatusInternalServerError, "failed to update device keys")
			return
		}
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

	if err := h.mailer.SendDeviceConfig(r.Context(), userEmail, deviceName, configText); err != nil {
		h.log.Error("failed to send config email", "error", err, "device_id", id, "email", userEmail)
		respondError(w, http.StatusInternalServerError, "failed to send configuration email")
		return
	}

	h.log.Info("config email sent", "device_id", id, "email", userEmail)
	respondJSON(w, http.StatusOK, map[string]string{"status": "sent", "email": userEmail})
}
