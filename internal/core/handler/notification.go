package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// NotificationHandler provides endpoints for in-app notifications
// sourced from the audit_log table.
type NotificationHandler struct {
	pool *pgxpool.Pool
}

// NewNotificationHandler creates a NotificationHandler backed by the given connection pool.
func NewNotificationHandler(pool *pgxpool.Pool) *NotificationHandler {
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

// notificationActions defines which audit actions are surfaced as notifications.
var notificationActions = []string{
	"CREATE", "DELETE", "UPDATE",
	"device.approved", "device.revoked",
	"login", "logout", "mfa_failed",
	"gateway.connected", "gateway.disconnected",
}

// list returns recent notifications (audit events) for the current user.
// Admins see all events; regular users see only their own.
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

	var query string
	var args []any

	if claims.IsAdmin {
		query = `SELECT id, timestamp, action, resource, details, user_id::text
			FROM audit_log
			ORDER BY timestamp DESC
			LIMIT $1`
		args = []any{limit}
	} else {
		query = `SELECT id, timestamp, action, resource, details, user_id::text
			FROM audit_log
			WHERE user_id = $1
			ORDER BY timestamp DESC
			LIMIT $2`
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

// unreadCount returns the number of audit events newer than the provided `since` timestamp.
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

	if claims.IsAdmin {
		err = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM audit_log WHERE timestamp > $1`, since).Scan(&count)
	} else {
		err = h.pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM audit_log WHERE user_id = $1 AND timestamp > $2`,
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

// markRead is a no-op endpoint that the frontend calls to record the "last read" timestamp.
// The actual tracking is done client-side via localStorage, but this endpoint exists
// for future server-side tracking.
func (h *NotificationHandler) markRead(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
