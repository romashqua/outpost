package handler

import (
	"context"
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
	pool DB
	log      *slog.Logger
	notifier PeerNotifier
	mailer   DeviceMailer
}

// NewDeviceHandler creates a DeviceHandler backed by the given connection pool.
func NewDeviceHandler(pool DB, logger ...*slog.Logger) *DeviceHandler {
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
		r.Put("/", h.update)
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
	OwnerName       string     `json:"owner_name"`
	Name            string     `json:"name"`
	WireguardPubkey string     `json:"wireguard_pubkey"`
	AssignedIP      string     `json:"assigned_ip"`
	IsApproved      bool       `json:"is_approved"`
	LastHandshake   *time.Time `json:"last_handshake,omitempty"`
	NetworkID       *uuid.UUID `json:"network_id,omitempty"`
	NetworkName     *string    `json:"network_name,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type createDeviceRequest struct {
	Name            string `json:"name"`
	WireguardPubkey string `json:"wireguard_pubkey"`
	UserID          string `json:"user_id"`
	NetworkID       string `json:"network_id"`
}

type enrollRequest struct {
	Name            string `json:"name"`
	WireguardPubkey string `json:"wireguard_pubkey"`
}

type gatewayEndpointResponse struct {
	ID              string `json:"id"`
	Endpoint        string `json:"endpoint"`
	ServerPublicKey string `json:"server_public_key"`
	Priority        int    `json:"priority"`
	HealthStatus    string `json:"health_status"`
}

type enrollResponse struct {
	DeviceID           uuid.UUID                 `json:"device_id"`
	Address            string                    `json:"address"`
	DNS                []string                  `json:"dns"`
	Endpoint           string                    `json:"endpoint"`
	ServerPublicKey    string                    `json:"server_public_key"`
	AllowedIPs         []string                  `json:"allowed_ips"`
	PersistentKeepalive int                      `json:"persistent_keepalive"`
	Gateways           []gatewayEndpointResponse `json:"gateways,omitempty"`
}

type configResponse struct {
	Config     string                    `json:"config"`
	PrivateKey string                    `json:"private_key"`
	PublicKey  string                    `json:"public_key"`
	Gateways   []gatewayEndpointResponse `json:"gateways,omitempty"`
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
		`SELECT d.id, d.user_id, COALESCE(u.username, ''), d.name, d.wireguard_pubkey, host(d.assigned_ip), d.is_approved, d.last_handshake, d.network_id, n.name, d.created_at, d.updated_at
		 FROM devices d
		 LEFT JOIN networks n ON n.id = d.network_id
		 LEFT JOIN users u ON u.id = d.user_id
		 ORDER BY d.created_at DESC
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
		if err := rows.Scan(&d.ID, &d.UserID, &d.OwnerName, &d.Name, &d.WireguardPubkey,
			&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.NetworkID, &d.NetworkName, &d.CreatedAt, &d.UpdatedAt); err != nil {
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
		`SELECT d.id, d.user_id, COALESCE(u.username, ''), d.name, d.wireguard_pubkey, host(d.assigned_ip), d.is_approved, d.last_handshake, d.network_id, n.name, d.created_at, d.updated_at
		 FROM devices d
		 LEFT JOIN networks n ON n.id = d.network_id
		 LEFT JOIN users u ON u.id = d.user_id
		 WHERE d.user_id = $1
		 ORDER BY d.created_at DESC
		 LIMIT $2 OFFSET $3`, userID, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query devices")
		return
	}
	defer rows.Close()

	devices := make([]deviceResponse, 0)
	for rows.Next() {
		var d deviceResponse
		if err := rows.Scan(&d.ID, &d.UserID, &d.OwnerName, &d.Name, &d.WireguardPubkey,
			&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.NetworkID, &d.NetworkName, &d.CreatedAt, &d.UpdatedAt); err != nil {
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
	// If network_id is specified, use that network; otherwise pick the first active one.
	const maxRetries = 5
	var d deviceResponse

	// Resolve network and its allocation CIDR (tunnel_cidr if set, else address).
	var createNetID uuid.UUID
	var createAllocCIDR string
	if req.NetworkID != "" {
		err = h.pool.QueryRow(r.Context(),
			`SELECT id, COALESCE(tunnel_cidr, address)::text FROM networks WHERE id = $1 AND is_active = true`,
			req.NetworkID,
		).Scan(&createNetID, &createAllocCIDR)
	} else {
		err = h.pool.QueryRow(r.Context(),
			`SELECT id, COALESCE(tunnel_cidr, address)::text FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
		).Scan(&createNetID, &createAllocCIDR)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusUnprocessableEntity, "no active network found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to read network")
		}
		return
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		err = h.pool.QueryRow(r.Context(),
			`WITH alloc AS (
				SELECT $5::cidr AS cidr, $6::uuid AS net_id
			),
			candidate AS (
				SELECT host(network(alloc.cidr) + s.off)::inet AS ip, alloc.net_id
				FROM alloc, generate_series(2, (1 << (32 - masklen(alloc.cidr))) - 2) AS s(off)
				WHERE NOT EXISTS (
					SELECT 1 FROM devices WHERE assigned_ip = host(network(alloc.cidr) + s.off)::inet
				)
				LIMIT 1
			)
			INSERT INTO devices (user_id, name, wireguard_pubkey, wireguard_privkey, assigned_ip, network_id)
			SELECT $1, $2, $3, $4, candidate.ip, candidate.net_id
			FROM candidate
			RETURNING id, user_id, name, wireguard_pubkey, host(assigned_ip), is_approved, last_handshake, created_at, updated_at`,
			userID, req.Name, req.WireguardPubkey, ptrOrNil(generatedPrivKey), createAllocCIDR, createNetID,
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

	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	// Combine ownership check into SQL to prevent timing-based device enumeration.
	var d deviceResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT d.id, d.user_id, COALESCE(u.username, ''), d.name, d.wireguard_pubkey, host(d.assigned_ip), d.is_approved, d.last_handshake, d.created_at, d.updated_at
		 FROM devices d
		 LEFT JOIN users u ON u.id = d.user_id
		 WHERE d.id = $1 AND (d.user_id = $2 OR $3 = true)`, id, claims.UserID, claims.IsAdmin,
	).Scan(&d.ID, &d.UserID, &d.OwnerName, &d.Name, &d.WireguardPubkey,
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

type updateDeviceRequest struct {
	Name *string `json:"name,omitempty"`
}

// @Summary Update device
// @Description Update a device's name. Non-admins can only update their own devices.
// @Tags Devices
// @Accept json
// @Produce json
// @Param id path string true "Device ID (UUID)"
// @Param body body updateDeviceRequest true "Update data"
// @Success 200 {object} deviceResponse
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /devices/{id} [put]
func (h *DeviceHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateDeviceRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == nil {
		respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}

	// Verify ownership: non-admins can only update their own devices.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var d deviceResponse
	err = h.pool.QueryRow(r.Context(),
		`UPDATE devices SET name = COALESCE($1, name), updated_at = now()
		 WHERE id = $2 AND (user_id = $3 OR $4 = true)
		 RETURNING id, user_id, name, wireguard_pubkey, assigned_ip, is_approved,
		           last_handshake, network_id, created_at, updated_at`,
		req.Name, id, claims.UserID, claims.IsAdmin,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey, &d.AssignedIP,
		&d.IsApproved, &d.LastHandshake, &d.NetworkID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		h.log.Error("failed to update device", "error", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to update device")
		return
	}

	h.log.Info("device updated", "id", id, "name", *req.Name)
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

	// Fetch device owner and pubkey — ownership check in SQL to prevent timing leaks.
	var ownerID, pubkey string
	err = h.pool.QueryRow(r.Context(),
		`SELECT user_id::text, wireguard_pubkey FROM devices WHERE id = $1 AND (user_id = $2 OR $3 = true)`, id, claims.UserID, claims.IsAdmin,
	).Scan(&ownerID, &pubkey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "device not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch device")
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
// @Description Self-service device enrollment. Auto-approves and returns connection parameters including a list of all healthy gateways for HA failover (sorted by priority DESC). The primary gateway is also returned in the top-level endpoint/server_public_key fields for backward compatibility.
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

	// Find all active gateways with their networks for enrollment (HA support).
	var err error
	var networkCIDR string
	var tunnelCIDR *string
	var networkDNS []string
	var networkID uuid.UUID
	var gatewayEndpoint, gatewayPubkey string
	var gatewayPublicIP *string
	var networkPort int

	// Fetch all active gateways for HA failover list.
	gwRows, err := h.pool.Query(r.Context(),
		`SELECT g.id::text, g.endpoint, g.wireguard_pubkey, host(g.public_ip),
		        g.priority, g.health_status,
		        n.id, n.address::text, n.tunnel_cidr::text, n.dns, n.port
		 FROM gateways g
		 JOIN gateway_networks gn ON gn.gateway_id = g.id
		 JOIN networks n ON n.id = gn.network_id AND n.is_active = true
		 WHERE g.is_active = true
		 ORDER BY g.priority DESC, g.created_at`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read gateway/network configuration")
		return
	}
	defer gwRows.Close()

	var allGateways []gatewayEndpointResponse
	var primarySet bool
	for gwRows.Next() {
		var gwID, gwEndpoint, gwPubkey string
		var gwPublicIP *string
		var gwPriority int
		var gwHealth string
		var nID uuid.UUID
		var nCIDR string
		var nTunnelCIDR *string
		var nDNS []string
		var nPort int
		if err := gwRows.Scan(&gwID, &gwEndpoint, &gwPubkey, &gwPublicIP,
			&gwPriority, &gwHealth,
			&nID, &nCIDR, &nTunnelCIDR, &nDNS, &nPort); err != nil {
			continue
		}
		// Build client-facing endpoint.
		ep := gwEndpoint
		if gwPublicIP != nil && *gwPublicIP != "" {
			ep = fmt.Sprintf("%s:%d", *gwPublicIP, nPort)
		}
		allGateways = append(allGateways, gatewayEndpointResponse{
			ID:              gwID,
			Endpoint:        ep,
			ServerPublicKey: gwPubkey,
			Priority:        gwPriority,
			HealthStatus:    gwHealth,
		})
		// Use first (highest priority) as primary for backward compat.
		if !primarySet {
			gatewayEndpoint = gwEndpoint
			gatewayPubkey = gwPubkey
			gatewayPublicIP = gwPublicIP
			networkID = nID
			networkCIDR = nCIDR
			tunnelCIDR = nTunnelCIDR
			networkDNS = nDNS
			networkPort = nPort
			primarySet = true
		}
	}
	if !primarySet {
		respondError(w, http.StatusUnprocessableEntity, "no active gateway with an active network — create and activate a gateway first")
		return
	}

	// Determine IP allocation subnet: tunnel_cidr (VPN overlay) or address (legacy).
	allocCIDR := networkCIDR
	if tunnelCIDR != nil && *tunnelCIDR != "" {
		allocCIDR = *tunnelCIDR
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// For client-facing endpoint, prefer public_ip + network port over internal endpoint.
	clientEndpoint := gatewayEndpoint
	if gatewayPublicIP != nil && *gatewayPublicIP != "" {
		clientEndpoint = fmt.Sprintf("%s:%d", *gatewayPublicIP, networkPort)
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
	// Uses a CTE that finds the first unused IP in the allocation subnet,
	// with a retry loop to handle concurrent inserts racing for the same IP.
	// allocCIDR is either tunnel_cidr (VPN overlay) or address (legacy single-subnet).
	const maxEnrollRetries = 5
	var deviceID uuid.UUID
	var assignedIP string
	for attempt := 0; attempt < maxEnrollRetries; attempt++ {
		err = h.pool.QueryRow(r.Context(),
			`WITH alloc AS (
				SELECT $4::cidr AS cidr, $5::uuid AS net_id
			),
			candidate AS (
				SELECT host(network(alloc.cidr) + s.off)::inet AS ip, alloc.net_id
				FROM alloc, generate_series(2, (1 << (32 - masklen(alloc.cidr))) - 2) AS s(off)
				WHERE NOT EXISTS (
					SELECT 1 FROM devices WHERE assigned_ip = host(network(alloc.cidr) + s.off)::inet
				)
				LIMIT 1
			)
			INSERT INTO devices (user_id, name, wireguard_pubkey, assigned_ip, is_approved, network_id)
			SELECT $1, $2, $3, candidate.ip, true, candidate.net_id
			FROM candidate
			RETURNING id, host(assigned_ip)`,
			userID, req.Name, req.WireguardPubkey, allocCIDR, networkID,
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
			if strings.Contains(pgErr.ConstraintName, "pubkey") {
				// Device with this pubkey already enrolled — return existing config (idempotent re-enroll).
				var existingID uuid.UUID
				var existingIP string
				reErr := h.pool.QueryRow(r.Context(),
					`SELECT id, host(assigned_ip) FROM devices WHERE wireguard_pubkey = $1`,
					req.WireguardPubkey,
				).Scan(&existingID, &existingIP)
				if reErr != nil {
					respondError(w, http.StatusInternalServerError, "failed to look up existing device")
					return
				}
				_, allocNet2, _ := net.ParseCIDR(allocCIDR)
				allocMask2, _ := allocNet2.Mask.Size()
				resp := enrollResponse{
					DeviceID:            existingID,
					Address:             fmt.Sprintf("%s/%d", existingIP, allocMask2),
					DNS:                 networkDNS,
					Endpoint:            clientEndpoint,
					ServerPublicKey:     gatewayPubkey,
					AllowedIPs:          []string{networkCIDR},
					PersistentKeepalive: 25,
					Gateways:            allGateways,
				}
				h.log.Info("device re-enrolled (existing pubkey)", "device_id", existingID, "address", existingIP)
				respondJSON(w, http.StatusOK, resp)
				return
			}
			respondError(w, http.StatusConflict, "device with this name already exists")
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

	// Compute mask length from the allocation CIDR for the client address.
	_, allocNet, _ := net.ParseCIDR(allocCIDR)
	allocMask, _ := allocNet.Mask.Size()
	clientAddress := fmt.Sprintf("%s/%d", assignedIP, allocMask)

	resp := enrollResponse{
		DeviceID:            deviceID,
		Address:             clientAddress,
		DNS:                 networkDNS,
		Endpoint:            clientEndpoint,
		ServerPublicKey:     gatewayPubkey,
		AllowedIPs:          []string{networkCIDR},
		PersistentKeepalive: 25,
		Gateways:            allGateways,
	}

	// Notify gateways about the new peer (enroll auto-approves).
	if h.notifier != nil && req.WireguardPubkey != "" {
		h.notifier.NotifyPeerAdd(req.WireguardPubkey, []string{assignedIP + "/32"})
	}

	h.log.Info("device enrolled", "device_id", deviceID, "name", req.Name, "address", clientAddress, "gateway", gatewayEndpoint)
	respondJSON(w, http.StatusCreated, resp)
}

// @Summary Download device config
// @Description Generate and return a WireGuard configuration file for the device. Includes all healthy gateways for the device's network for HA failover support.
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
	var deviceNetworkID *string
	err = h.pool.QueryRow(r.Context(),
		`SELECT name, host(assigned_ip), user_id::text, network_id::text FROM devices WHERE id = $1`, id,
	).Scan(&deviceName, &assignedIP, &ownerID, &deviceNetworkID)
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

	// Get network info: address (target network for AllowedIPs) and tunnel_cidr (for client Address).
	var networkCIDR string
	var dlTunnelCIDR *string
	var networkDNS []string
	if deviceNetworkID != nil {
		err = h.pool.QueryRow(r.Context(),
			`SELECT address::text, tunnel_cidr::text, dns FROM networks WHERE id = $1`, *deviceNetworkID,
		).Scan(&networkCIDR, &dlTunnelCIDR, &networkDNS)
	} else {
		err = h.pool.QueryRow(r.Context(),
			`SELECT address::text, tunnel_cidr::text, dns FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
		).Scan(&networkCIDR, &dlTunnelCIDR, &networkDNS)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read network mask")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	// Get all active gateways for this device's network (HA support).
	// Prefer gateways that serve this device's network, fallback to any active gateway.
	var gatewayEndpoint, gatewayPubkey string
	var dlClientEndpoint string
	var dlGateways []gatewayEndpointResponse

	gwQuery := `SELECT g.id::text, g.endpoint, g.wireguard_pubkey, host(g.public_ip),
	                   g.priority, g.health_status, COALESCE(n.port, 51820)
	            FROM gateways g
	            JOIN gateway_networks gn ON gn.gateway_id = g.id
	            JOIN networks n ON n.id = gn.network_id
	            WHERE gn.network_id = $1 AND g.is_active = true
	            ORDER BY g.priority DESC, g.created_at`
	var gwArgs []any
	if deviceNetworkID != nil {
		gwArgs = []any{*deviceNetworkID}
	} else {
		gwQuery = `SELECT g.id::text, g.endpoint, g.wireguard_pubkey, host(g.public_ip),
		                  g.priority, g.health_status, COALESCE(n.port, 51820)
		           FROM gateways g
		           LEFT JOIN networks n ON n.id = g.network_id
		           WHERE g.is_active = true
		           ORDER BY g.priority DESC, g.created_at`
		gwArgs = nil
	}
	gwRows, gwErr := h.pool.Query(r.Context(), gwQuery, gwArgs...)
	if gwErr != nil {
		respondError(w, http.StatusInternalServerError, "failed to read gateway configuration")
		return
	}
	for gwRows.Next() {
		var gwID, gwEndpoint, gwPubkey string
		var gwPublicIP *string
		var gwPriority int
		var gwHealth string
		var gwPort int
		if err := gwRows.Scan(&gwID, &gwEndpoint, &gwPubkey, &gwPublicIP,
			&gwPriority, &gwHealth, &gwPort); err != nil {
			continue
		}
		ep := gwEndpoint
		if gwPublicIP != nil && *gwPublicIP != "" {
			if gwPort == 0 {
				gwPort = 51820
			}
			ep = fmt.Sprintf("%s:%d", *gwPublicIP, gwPort)
		}
		dlGateways = append(dlGateways, gatewayEndpointResponse{
			ID:              gwID,
			Endpoint:        ep,
			ServerPublicKey: gwPubkey,
			Priority:        gwPriority,
			HealthStatus:    gwHealth,
		})
		// Use first (highest priority) as primary.
		if gatewayEndpoint == "" {
			gatewayEndpoint = gwEndpoint
			gatewayPubkey = gwPubkey
			dlClientEndpoint = ep
		}
	}
	gwRows.Close()
	if gatewayEndpoint == "" {
		respondError(w, http.StatusUnprocessableEntity, "no active gateway available — create and activate a gateway first")
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
		// Device was enrolled with a user-provided public key — the server
		// does not have the private key. Config download is only available
		// for devices with auto-generated keys.
		respondError(w, http.StatusUnprocessableEntity,
			"config download unavailable: this device was enrolled with a user-provided key. "+
				"Re-create the device with auto-generated keys or use your own WireGuard config file.")
		return
	}

	// Compute mask from tunnel_cidr (if set) or network address.
	dlAllocCIDR := networkCIDR
	if dlTunnelCIDR != nil && *dlTunnelCIDR != "" {
		dlAllocCIDR = *dlTunnelCIDR
	}
	_, dlAllocNet, _ := net.ParseCIDR(dlAllocCIDR)
	dlMaskLen, _ := dlAllocNet.Mask.Size()
	clientAddress := fmt.Sprintf("%s/%d", assignedIP, dlMaskLen)

	// Build AllowedIPs based on user's group ACLs.
	// If user has ACL entries, only include allowed networks/CIDRs.
	// If no ACLs defined, fall back to the device's network CIDR.
	allowedIPs := h.resolveAllowedIPs(r.Context(), ownerID, networkCIDR)

	cfg := wireguard.InterfaceConfig{
		PrivateKey: privateKey,
		Address:    clientAddress,
		DNS:        networkDNS,
		Peers: []wireguard.PeerConfig{
			{
				PublicKey:           gatewayPubkey,
				AllowedIPs:          allowedIPs,
				Endpoint:            dlClientEndpoint,
				PersistentKeepalive: 25,
			},
		},
	}

	configText := wireguard.RenderConfig(cfg)

	resp := configResponse{
		Config:     configText,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		Gateways:   dlGateways,
	}

	respondJSON(w, http.StatusOK, resp)
}

// resolveAllowedIPs determines which network CIDRs a user's device should be
// able to reach. It checks the user's group memberships and their network ACLs.
// If the user has specific ACL entries, only those networks/CIDRs are included.
// If no ACLs are defined for the user's groups, falls back to the device's
// network CIDR (full access to own network).
// This works identically for standard WireGuard clients and outpost-client.
func (h *DeviceHandler) resolveAllowedIPs(ctx context.Context, userID string, fallbackCIDR string) []string {
	// Query: get all allowed CIDRs from network_acls for groups the user belongs to.
	// Uses LATERAL unnest to avoid set-returning functions inside CASE.
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT cidr FROM (
		     -- Explicit allowed_ips entries
		     SELECT unnest(a.allowed_ips)::text AS cidr
		     FROM network_acls a
		     JOIN user_groups ug ON ug.group_id = a.group_id
		     JOIN networks n ON n.id = a.network_id
		     WHERE ug.user_id = $1::uuid
		       AND n.is_active = true
		       AND a.allowed_ips != '{0.0.0.0/0}'
		     UNION
		     -- Wildcard entries: use network CIDR
		     SELECT n.address::text AS cidr
		     FROM network_acls a
		     JOIN user_groups ug ON ug.group_id = a.group_id
		     JOIN networks n ON n.id = a.network_id
		     WHERE ug.user_id = $1::uuid
		       AND n.is_active = true
		       AND a.allowed_ips = '{0.0.0.0/0}'
		 ) sub
		 ORDER BY cidr`, userID)
	if err != nil {
		h.log.Error("failed to resolve ACL allowed IPs", "error", err, "user_id", userID)
		return []string{fallbackCIDR}
	}
	defer rows.Close()

	var cidrs []string
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			continue
		}
		cidrs = append(cidrs, cidr)
	}

	// No ACL entries → fallback to device's network CIDR.
	if len(cidrs) == 0 {
		return []string{fallbackCIDR}
	}
	return cidrs
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
	var sendDeviceNetworkID *string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT host(assigned_ip), network_id::text FROM devices WHERE id = $1`, id,
	).Scan(&assignedIP, &sendDeviceNetworkID)

	var networkCIDR string
	var scTunnelCIDR *string
	var networkDNS []string
	if sendDeviceNetworkID != nil {
		err = h.pool.QueryRow(r.Context(),
			`SELECT address::text, tunnel_cidr::text, dns FROM networks WHERE id = $1`, *sendDeviceNetworkID,
		).Scan(&networkCIDR, &scTunnelCIDR, &networkDNS)
	} else {
		err = h.pool.QueryRow(r.Context(),
			`SELECT address::text, tunnel_cidr::text, dns FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
		).Scan(&networkCIDR, &scTunnelCIDR, &networkDNS)
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read network configuration")
		return
	}
	if len(networkDNS) == 0 {
		networkDNS = []string{"1.1.1.1", "8.8.8.8"}
	}

	var scGwEndpoint, scGwPubkey string
	var scGwPublicIP *string
	var scNetPort int
	err = h.pool.QueryRow(r.Context(),
		`SELECT g.endpoint, g.wireguard_pubkey, host(g.public_ip), COALESCE(n.port, 51820)
		 FROM gateways g JOIN networks n ON n.id = g.network_id
		 WHERE n.is_active = true AND g.is_active = true
		 ORDER BY g.priority DESC, g.created_at LIMIT 1`,
	).Scan(&scGwEndpoint, &scGwPubkey, &scGwPublicIP, &scNetPort)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read gateway configuration")
		return
	}
	scEndpoint := scGwEndpoint
	if scGwPublicIP != nil && *scGwPublicIP != "" {
		scEndpoint = fmt.Sprintf("%s:%d", *scGwPublicIP, scNetPort)
	}

	var storedPrivKey *string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT wireguard_privkey FROM devices WHERE id = $1`, id,
	).Scan(&storedPrivKey)

	var privateKey string
	if storedPrivKey != nil && *storedPrivKey != "" {
		privateKey = *storedPrivKey
	} else {
		respondError(w, http.StatusUnprocessableEntity,
			"config email unavailable: this device was enrolled with a user-provided key")
		return
	}

	scAllocCIDR := networkCIDR
	if scTunnelCIDR != nil && *scTunnelCIDR != "" {
		scAllocCIDR = *scTunnelCIDR
	}
	_, scAllocNet, _ := net.ParseCIDR(scAllocCIDR)
	scMaskLen, _ := scAllocNet.Mask.Size()
	clientAddress := fmt.Sprintf("%s/%d", assignedIP, scMaskLen)
	cfg := wireguard.InterfaceConfig{
		PrivateKey: privateKey,
		Address:    clientAddress,
		DNS:        networkDNS,
		Peers: []wireguard.PeerConfig{
			{
				PublicKey:           scGwPubkey,
				AllowedIPs:          []string{networkCIDR},
				Endpoint:            scEndpoint,
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
