package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceHandler provides endpoints for managing WireGuard devices (peers).
type DeviceHandler struct {
	pool *pgxpool.Pool
}

// NewDeviceHandler creates a DeviceHandler backed by the given connection pool.
func NewDeviceHandler(pool *pgxpool.Pool) *DeviceHandler {
	return &DeviceHandler{pool: pool}
}

// Routes returns a chi.Router with device management endpoints mounted.
func (h *DeviceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/my", h.listMy)
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
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
}

func (h *DeviceHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, user_id, name, wireguard_pubkey, assigned_ip, is_approved, last_handshake, created_at, updated_at
		 FROM devices
		 ORDER BY created_at DESC`)
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
		`SELECT id, user_id, name, wireguard_pubkey, assigned_ip, is_approved, last_handshake, created_at, updated_at
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
	if req.WireguardPubkey == "" {
		respondError(w, http.StatusBadRequest, "wireguard_pubkey is required")
		return
	}

	// In production, user_id comes from the authenticated session.
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

	// Assign the next available IP from the first active network.
	// This is a simplified allocation; production should use proper IPAM.
	var assignedIP string
	err = h.pool.QueryRow(r.Context(),
		`SELECT host(network(address) + (SELECT COUNT(*) + 2 FROM devices))
		 FROM networks WHERE is_active = true ORDER BY created_at LIMIT 1`,
	).Scan(&assignedIP)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to allocate IP address")
		return
	}

	var d deviceResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO devices (user_id, name, wireguard_pubkey, assigned_ip)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, name, wireguard_pubkey, assigned_ip, is_approved, last_handshake, created_at, updated_at`,
		userID, req.Name, req.WireguardPubkey, assignedIP,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
		&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusConflict, "device already exists or invalid data")
		return
	}

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
		`SELECT id, user_id, name, wireguard_pubkey, assigned_ip, is_approved, last_handshake, created_at, updated_at
		 FROM devices WHERE id = $1`, id,
	).Scan(&d.ID, &d.UserID, &d.Name, &d.WireguardPubkey,
		&d.AssignedIP, &d.IsApproved, &d.LastHandshake, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "device not found")
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
		respondError(w, http.StatusInternalServerError, "failed to delete device")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "device not found")
		return
	}

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
