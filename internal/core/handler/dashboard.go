package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DashboardHandler struct {
	pool *pgxpool.Pool
}

func NewDashboardHandler(pool *pgxpool.Pool) *DashboardHandler {
	return &DashboardHandler{pool: pool}
}

func (h *DashboardHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/stats", h.stats)
	return r
}

type dashboardStats struct {
	ActiveUsers     int `json:"active_users"`
	TotalUsers      int `json:"total_users"`
	ActiveDevices   int `json:"active_devices"`
	TotalDevices    int `json:"total_devices"`
	ActiveGateways  int `json:"active_gateways"`
	TotalGateways   int `json:"total_gateways"`
	ActiveNetworks  int `json:"active_networks"`
	S2STunnels      int `json:"s2s_tunnels"`
}

func (h *DashboardHandler) stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var s dashboardStats

	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE is_active = true`).Scan(&s.ActiveUsers)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&s.TotalUsers)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE is_approved = true`).Scan(&s.ActiveDevices)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&s.TotalDevices)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM gateways WHERE is_active = true`).Scan(&s.ActiveGateways)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM gateways`).Scan(&s.TotalGateways)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM networks WHERE is_active = true`).Scan(&s.ActiveNetworks)
	_ = h.pool.QueryRow(ctx, `SELECT COUNT(*) FROM s2s_tunnels WHERE is_active = true`).Scan(&s.S2STunnels)

	respondJSON(w, http.StatusOK, s)
}
