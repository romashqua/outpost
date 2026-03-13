package analytics

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler provides analytics API endpoints.
type Handler struct {
	collector *Collector
}

// NewHandler creates an analytics Handler backed by the given connection pool.
func NewHandler(pool *pgxpool.Pool) *Handler {
	return &Handler{
		collector: NewCollector(pool),
	}
}

// Routes returns a chi.Router with analytics endpoints mounted.
//
//	GET /bandwidth          - bandwidth over time
//	GET /top-users          - top users by traffic
//	GET /connections-heatmap - connections by hour
//	GET /summary            - dashboard summary stats
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/bandwidth", h.bandwidth)
	r.Get("/top-users", h.topUsers)
	r.Get("/connections-heatmap", h.connectionsHeatmap)
	r.Get("/summary", h.summary)
	return r
}

func (h *Handler) bandwidth(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseTimeRange(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	bucketParam := r.URL.Query().Get("bucket")
	bucketSize := time.Hour
	if bucketParam != "" {
		d, err := time.ParseDuration(bucketParam)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid bucket duration")
			return
		}
		bucketSize = d
	}

	data, err := h.collector.BandwidthOverTime(r.Context(), from, to, bucketSize)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query bandwidth data")
		return
	}

	respondJSON(w, http.StatusOK, data)
}

func (h *Handler) topUsers(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseTimeRange(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	data, err := h.collector.TopUsers(r.Context(), from, to, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query top users")
		return
	}

	respondJSON(w, http.StatusOK, data)
}

func (h *Handler) connectionsHeatmap(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseTimeRange(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	data, err := h.collector.ConnectionsByHour(r.Context(), from, to)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query connections heatmap")
		return
	}

	respondJSON(w, http.StatusOK, data)
}

type summaryResponse struct {
	TotalRxBytes  int64 `json:"total_rx_bytes"`
	TotalTxBytes  int64 `json:"total_tx_bytes"`
	TotalFlows    int   `json:"total_flows"`
	UniqueUsers   int   `json:"unique_users"`
	UniqueDevices int   `json:"unique_devices"`
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseTimeRange(r)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var s summaryResponse
	err = h.collector.pool.QueryRow(r.Context(),
		`SELECT
			COALESCE(SUM(bytes_sent), 0),
			COALESCE(SUM(bytes_recv), 0),
			COUNT(*),
			COUNT(DISTINCT user_id),
			COUNT(DISTINCT device_id)
		 FROM flow_records
		 WHERE recorded_at >= $1 AND recorded_at < $2`,
		from, to,
	).Scan(&s.TotalTxBytes, &s.TotalRxBytes, &s.TotalFlows, &s.UniqueUsers, &s.UniqueDevices)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query summary")
		return
	}

	respondJSON(w, http.StatusOK, s)
}

// parseTimeRange extracts "from" and "to" query params as RFC3339 timestamps.
// Defaults to last 24 hours if not provided.
func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = t
	}

	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to = t
	}

	return from, to, nil
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message, "message": message})
}
