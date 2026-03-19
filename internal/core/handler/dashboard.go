package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type DashboardHandler struct {
	pool DB
	log  *slog.Logger
}

func NewDashboardHandler(pool DB) *DashboardHandler {
	return &DashboardHandler{pool: pool, log: slog.Default().With("handler", "dashboard")}
}

func (h *DashboardHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/stats", h.stats)
	return r
}

type dashboardStats struct {
	ActiveUsers    int `json:"active_users"`
	TotalUsers     int `json:"total_users"`
	ActiveDevices  int `json:"active_devices"`
	TotalDevices   int `json:"total_devices"`
	ActiveGateways int `json:"active_gateways"`
	TotalGateways  int `json:"total_gateways"`
	ActiveNetworks int `json:"active_networks"`
	S2STunnels     int `json:"s2s_tunnels"`
}

// @Summary Get dashboard stats
// @Description Returns aggregate statistics for users, devices, gateways, networks, and S2S tunnels.
// @Tags Dashboard
// @Produce json
// @Success 200 {object} dashboardStats
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /dashboard/stats [get]
func (h *DashboardHandler) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var s dashboardStats

	// Use a single query for efficiency and proper error handling.
	err := h.pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE is_active = true),
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM devices WHERE is_approved = true),
			(SELECT COUNT(*) FROM devices),
			(SELECT COUNT(*) FROM gateways WHERE is_active = true),
			(SELECT COUNT(*) FROM gateways),
			(SELECT COUNT(*) FROM networks WHERE is_active = true),
			(SELECT COUNT(*) FROM s2s_tunnels WHERE is_active = true)
	`).Scan(
		&s.ActiveUsers, &s.TotalUsers,
		&s.ActiveDevices, &s.TotalDevices,
		&s.ActiveGateways, &s.TotalGateways,
		&s.ActiveNetworks, &s.S2STunnels,
	)
	if err != nil {
		h.log.Error("failed to query dashboard stats", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to load dashboard stats")
		return
	}

	respondJSON(w, http.StatusOK, s)
}
