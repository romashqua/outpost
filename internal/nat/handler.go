package nat

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler provides HTTP endpoints for NAT traversal status and relay server management.
type Handler struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewHandler creates a new NAT traversal HTTP handler.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, logger: logger}
}

// Routes returns a Chi router with all NAT-related endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/status", h.getStatus)
	r.Post("/check", h.triggerCheck)
	r.Get("/relays", h.listRelays)
	r.Post("/relays", h.createRelay)
	r.Delete("/relays/{id}", h.deleteRelay)
	return r
}

// --- Response types ---

type relayServerResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Region    string    `json:"region"`
	Protocol  string    `json:"protocol"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type natStatusResponse struct {
	DeviceID      string    `json:"device_id"`
	NATType       string    `json:"nat_type"`
	ExternalIP    string    `json:"external_ip"`
	ExternalPort  int       `json:"external_port"`
	RelayServerID *string   `json:"relay_server_id,omitempty"`
	LastChecked   time.Time `json:"last_checked"`
	CreatedAt     time.Time `json:"created_at"`
}

type natCheckRequest struct {
	DeviceID    string `json:"device_id"`
	STUNServer1 string `json:"stun_server_1,omitempty"`
	STUNServer2 string `json:"stun_server_2,omitempty"`
}

type natCheckResponse struct {
	NATType      string `json:"nat_type"`
	ExternalIP   string `json:"external_ip"`
	ExternalPort int    `json:"external_port"`
}

// --- Handlers ---

// getStatus returns the latest NAT status for a device.
// GET /api/v1/nat/status?device_id=<uuid>
func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	deviceIDStr := r.URL.Query().Get("device_id")
	if deviceIDStr == "" {
		respondError(w, http.StatusBadRequest, "device_id query parameter is required")
		return
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid device_id UUID")
		return
	}

	var ns natStatusResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT device_id, nat_type, external_ip, external_port, relay_server_id, last_checked, created_at
		 FROM nat_status WHERE device_id = $1`, deviceID,
	).Scan(&ns.DeviceID, &ns.NATType, &ns.ExternalIP, &ns.ExternalPort, &ns.RelayServerID, &ns.LastChecked, &ns.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "no NAT status found for this device")
			return
		}
		h.logger.Error("failed to query NAT status", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to query NAT status")
		return
	}

	respondJSON(w, http.StatusOK, ns)
}

// triggerCheck runs NAT type detection and persists the result.
// POST /api/v1/nat/check
func (h *Handler) triggerCheck(w http.ResponseWriter, r *http.Request) {
	var req natCheckRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceID == "" {
		respondError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid device_id UUID")
		return
	}

	// Use default public STUN servers if not specified.
	server1 := req.STUNServer1
	if server1 == "" {
		server1 = "stun.l.google.com:19302"
	}
	server2 := req.STUNServer2
	if server2 == "" {
		server2 = "stun1.l.google.com:19302"
	}

	result, err := Discover(server1, server2)
	if err != nil {
		h.logger.Warn("NAT discovery failed", "err", err, "device_id", deviceID)
		// Still proceed with the result — it may have partial info.
	}

	// Upsert into nat_status (UNIQUE on device_id).
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO nat_status (device_id, nat_type, external_ip, external_port, last_checked)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (device_id) DO UPDATE
		 SET nat_type = EXCLUDED.nat_type,
		     external_ip = EXCLUDED.external_ip,
		     external_port = EXCLUDED.external_port,
		     last_checked = now()`,
		deviceID, string(result.NATType), result.ExternalIP, result.ExternalPort,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusBadRequest, "device not found")
			return
		}
		h.logger.Error("failed to upsert NAT status", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to save NAT status")
		return
	}

	respondJSON(w, http.StatusOK, natCheckResponse{
		NATType:      string(result.NATType),
		ExternalIP:   result.ExternalIP,
		ExternalPort: result.ExternalPort,
	})
}

// listRelays returns all active relay servers.
// GET /api/v1/nat/relays
func (h *Handler) listRelays(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, address, region, protocol, is_active, created_at, updated_at
		 FROM relay_servers ORDER BY created_at DESC`)
	if err != nil {
		h.logger.Error("failed to list relay servers", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to list relay servers")
		return
	}
	defer rows.Close()

	servers := make([]relayServerResponse, 0)
	for rows.Next() {
		var rs relayServerResponse
		if err := rows.Scan(&rs.ID, &rs.Name, &rs.Address, &rs.Region, &rs.Protocol, &rs.IsActive, &rs.CreatedAt, &rs.UpdatedAt); err != nil {
			h.logger.Error("failed to scan relay server", "err", err)
			respondError(w, http.StatusInternalServerError, "failed to scan relay server")
			return
		}
		servers = append(servers, rs)
	}
	if err := rows.Err(); err != nil {
		h.logger.Error("failed to iterate relay servers", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to iterate relay servers")
		return
	}

	respondJSON(w, http.StatusOK, servers)
}

// createRelay adds a new relay server (admin only).
// POST /api/v1/nat/relays
func (h *Handler) createRelay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Address  string `json:"address"`
		Region   string `json:"region"`
		Protocol string `json:"protocol"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Address == "" || req.Protocol == "" {
		respondError(w, http.StatusBadRequest, "name, address, and protocol are required")
		return
	}
	if req.Protocol != "stun" && req.Protocol != "turn" {
		respondError(w, http.StatusBadRequest, "protocol must be 'stun' or 'turn'")
		return
	}

	var rs relayServerResponse
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO relay_servers (name, address, region, protocol)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, address, region, protocol, is_active, created_at, updated_at`,
		req.Name, req.Address, req.Region, req.Protocol,
	).Scan(&rs.ID, &rs.Name, &rs.Address, &rs.Region, &rs.Protocol, &rs.IsActive, &rs.CreatedAt, &rs.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "relay server already exists")
			return
		}
		h.logger.Error("failed to create relay server", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to create relay server")
		return
	}

	respondJSON(w, http.StatusCreated, rs)
}

// deleteRelay removes a relay server (admin only).
// DELETE /api/v1/nat/relays/{id}
func (h *Handler) deleteRelay(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid UUID %q", idStr))
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM relay_servers WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusConflict, "relay server is referenced by NAT status entries")
			return
		}
		h.logger.Error("failed to delete relay server", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to delete relay server")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "relay server not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Helpers (local to nat package, matching handler patterns) ---

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message, "message": message})
}

func parseBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
