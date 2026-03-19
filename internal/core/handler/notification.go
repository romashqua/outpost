package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/romashqua/outpost/internal/auth"
)

// NotificationHandler provides endpoints for in-app notifications
// sourced from the audit_log table.
type NotificationHandler struct {
	pool DB
}

// NewNotificationHandler creates a NotificationHandler backed by the given connection pool.
func NewNotificationHandler(pool DB) *NotificationHandler {
	return &NotificationHandler{pool: pool}
}

// Routes returns a chi.Router with notification endpoints mounted.
func (h *NotificationHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Get("/unread-count", h.unreadCount)
	r.Post("/mark-read", h.markRead)
	return r
}

type notificationItem struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Details   any       `json:"details,omitempty"`
	UserID    *string   `json:"user_id,omitempty"`
}

// notificationActionPatterns defines SQL LIKE patterns for audit actions surfaced as notifications.
// Middleware logs actions as "METHOD /api/v1/...", semantic events use dotted names.
var notificationActionPatterns = []string{
	"POST /api/v1/auth/login",
	"POST /api/v1/auth/logout",
	"POST /api/v1/devices%",
	"DELETE /api/v1/devices%",
	"POST /api/v1/users%",
	"DELETE /api/v1/users%",
	"POST /api/v1/networks%",
	"DELETE /api/v1/networks%",
	"POST /api/v1/gateways%",
	"DELETE /api/v1/gateways%",
	"PUT /api/v1/settings%",
	"gateway.connected",
	"gateway.disconnected",
	"device.approved",
	"device.revoked",
}

// @Summary List notifications
// @Description Return recent audit-log events as notifications. Admins see all events; regular users see only their own.
// @Tags Notifications
// @Produce json
// @Param limit query int false "Max items to return (1-100, default 30)"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /notifications [get]
func (h *NotificationHandler) list(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := queryInt(r, "limit", 30)
	if limit < 1 || limit > 100 {
		limit = 30
	}

	// Build action filter from notification patterns.
	actionFilter := buildActionFilter()

	var query string
	var args []any

	if claims.IsAdmin {
		query = fmt.Sprintf(`SELECT id, timestamp, action, resource, details, user_id::text
			FROM audit_log
			WHERE (%s)
			ORDER BY timestamp DESC
			LIMIT $1`, actionFilter)
		args = []any{limit}
	} else {
		query = fmt.Sprintf(`SELECT id, timestamp, action, resource, details, user_id::text
			FROM audit_log
			WHERE user_id = $1 AND (%s)
			ORDER BY timestamp DESC
			LIMIT $2`, actionFilter)
		args = []any{claims.UserID, limit}
	}

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query notifications")
		return
	}
	defer rows.Close()

	items := make([]notificationItem, 0)
	for rows.Next() {
		var n notificationItem
		var detailsRaw []byte
		if err := rows.Scan(&n.ID, &n.Timestamp, &n.Action, &n.Resource, &detailsRaw, &n.UserID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan notification")
			return
		}
		if len(detailsRaw) > 0 {
			var details any
			if err := json.Unmarshal(detailsRaw, &details); err == nil {
				n.Details = details
			}
		}
		items = append(items, n)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate notifications")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"notifications": items,
		"total":         len(items),
	})
}

// @Summary Get unread notification count
// @Description Return the number of audit events newer than the given timestamp.
// @Tags Notifications
// @Produce json
// @Param since query string false "RFC3339 timestamp (default: 24h ago)"
// @Success 200 {object} map[string]any
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /notifications/unread-count [get]
func (h *NotificationHandler) unreadCount(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sinceStr := r.URL.Query().Get("since")
	since := time.Now().Add(-24 * time.Hour)
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	var count int
	var err error

	actionFilter := buildActionFilter()

	if claims.IsAdmin {
		err = h.pool.QueryRow(r.Context(),
			fmt.Sprintf(`SELECT COUNT(*) FROM audit_log WHERE timestamp > $1 AND (%s)`, actionFilter),
			since).Scan(&count)
	} else {
		err = h.pool.QueryRow(r.Context(),
			fmt.Sprintf(`SELECT COUNT(*) FROM audit_log WHERE user_id = $1 AND timestamp > $2 AND (%s)`, actionFilter),
			claims.UserID, since).Scan(&count)
	}

	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count notifications")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"count": count,
	})
}

// @Summary Mark notifications as read
// @Description Record that the user has seen notifications up to now. Currently a no-op placeholder for future server-side tracking.
// @Tags Notifications
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /notifications/mark-read [post]
func (h *NotificationHandler) markRead(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// buildActionFilter returns a SQL condition that matches only notification-worthy actions.
// The patterns use LIKE for prefix matching (e.g. "POST /api/v1/devices%")
// and exact match for semantic events (e.g. "gateway.connected").
func buildActionFilter() string {
	var conditions []string
	for _, p := range notificationActionPatterns {
		if strings.Contains(p, "%") {
			conditions = append(conditions, fmt.Sprintf("action LIKE '%s'", p))
		} else {
			conditions = append(conditions, fmt.Sprintf("action = '%s'", p))
		}
	}
	return strings.Join(conditions, " OR ")
}
