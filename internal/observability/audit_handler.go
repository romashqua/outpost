package observability

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// AuditEntry represents a single audit log row returned by the API.
type AuditEntry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	UserID    *string   `json:"user_id,omitempty"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Details   any       `json:"details,omitempty"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
}

// AuditListResponse wraps paginated audit log results.
type AuditListResponse struct {
	Data       []AuditEntry `json:"data"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PerPage    int          `json:"per_page"`
	TotalPages int          `json:"total_pages"`
}

// AuditStat represents an aggregated event count for a given action and time bucket.
type AuditStat struct {
	Action string `json:"action"`
	Bucket string `json:"bucket"`
	Count  int64  `json:"count"`
}

// AuditHandler serves audit log HTTP endpoints.
type AuditHandler struct {
	pool *pgxpool.Pool
}

// NewAuditHandler creates an AuditHandler backed by the given connection pool.
func NewAuditHandler(pool *pgxpool.Pool) *AuditHandler {
	return &AuditHandler{pool: pool}
}

// Routes returns a chi.Router with all audit log endpoints mounted.
func (h *AuditHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.With(auth.RequireAdmin).Get("/", h.listAuditLogs)
	r.With(auth.RequireAdmin).Get("/export", h.exportAuditLogs)
	r.With(auth.RequireAdmin).Get("/stats", h.auditStats)
	return r
}

// listAuditLogs handles GET / with pagination and filtering.
func (h *AuditHandler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	where, args := buildWhereClause(r)

	// Count total matching rows.
	countQuery := "SELECT COUNT(*) FROM audit_log" + where
	var total int64
	if err := h.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to count audit logs")
		return
	}

	// Fetch page.
	dataQuery := fmt.Sprintf(
		"SELECT id, timestamp, user_id, action, resource, details, ip_address, user_agent FROM audit_log%s ORDER BY timestamp DESC LIMIT $%d OFFSET $%d",
		where, len(args)+1, len(args)+2,
	)
	args = append(args, perPage, offset)

	rows, err := h.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to query audit logs")
		return
	}
	defer rows.Close()

	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action, &e.Resource, &detailsRaw, &e.IPAddress, &e.UserAgent); err != nil {
			respondAuditError(w, http.StatusInternalServerError, "failed to scan audit log row")
			return
		}
		if len(detailsRaw) > 0 {
			var details any
			if err := json.Unmarshal(detailsRaw, &details); err == nil {
				e.Details = details
			}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to iterate audit log rows")
		return
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	resp := AuditListResponse{
		Data:       entries,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	writeJSON(w, http.StatusOK, resp)
}

// exportAuditLogs handles GET /export, returning CSV or JSON based on ?format= query param.
func (h *AuditHandler) exportAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	where, args := buildWhereClause(r)
	// Limit export to 50,000 rows to prevent OOM on large audit logs.
	query := "SELECT id, timestamp, user_id, action, resource, details, ip_address, user_agent FROM audit_log" + where + " ORDER BY timestamp DESC LIMIT 50000"

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to query audit logs")
		return
	}
	defer rows.Close()

	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var e AuditEntry
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.UserID, &e.Action, &e.Resource, &detailsRaw, &e.IPAddress, &e.UserAgent); err != nil {
			respondAuditError(w, http.StatusInternalServerError, "failed to scan audit log row")
			return
		}
		if len(detailsRaw) > 0 {
			var details any
			if err := json.Unmarshal(detailsRaw, &details); err == nil {
				e.Details = details
			}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to iterate audit log rows")
		return
	}

	format := r.URL.Query().Get("format")
	if strings.EqualFold(format, "csv") {
		h.writeCSV(w, entries)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="audit_log.json"`)
	writeJSON(w, http.StatusOK, entries)
}

// writeCSV streams audit entries as CSV to the response.
func (h *AuditHandler) writeCSV(w http.ResponseWriter, entries []AuditEntry) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="audit_log.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"id", "timestamp", "user_id", "action", "resource", "details", "ip_address", "user_agent"})

	for _, e := range entries {
		userID := ""
		if e.UserID != nil {
			userID = *e.UserID
		}
		detailsStr := ""
		if e.Details != nil {
			if b, err := json.Marshal(e.Details); err == nil {
				detailsStr = string(b)
			}
		}
		_ = cw.Write([]string{
			strconv.FormatInt(e.ID, 10),
			e.Timestamp.Format(time.RFC3339),
			userID,
			e.Action,
			e.Resource,
			detailsStr,
			e.IPAddress,
			e.UserAgent,
		})
	}
}

// auditStats handles GET /stats, returning event counts grouped by action and time bucket.
func (h *AuditHandler) auditStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	where, args := buildWhereClause(r)

	query := fmt.Sprintf(
		`SELECT action, date_trunc('hour', timestamp) AS bucket, COUNT(*) AS count
		 FROM audit_log%s
		 GROUP BY action, bucket
		 ORDER BY bucket DESC, count DESC`,
		where,
	)

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to query audit stats")
		return
	}
	defer rows.Close()

	stats := make([]AuditStat, 0)
	for rows.Next() {
		var s AuditStat
		var bucket time.Time
		if err := rows.Scan(&s.Action, &bucket, &s.Count); err != nil {
			respondAuditError(w, http.StatusInternalServerError, "failed to scan audit stat row")
			return
		}
		s.Bucket = bucket.Format(time.RFC3339)
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		respondAuditError(w, http.StatusInternalServerError, "failed to iterate audit stat rows")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// buildWhereClause constructs a SQL WHERE clause from supported query parameters.
// Returns the clause string (prefixed with " WHERE " if non-empty) and positional args.
func buildWhereClause(r *http.Request) (string, []any) {
	var conditions []string
	var args []any
	idx := 1

	if v := r.URL.Query().Get("user_id"); v != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", idx))
		args = append(args, v)
		idx++
	}
	if v := r.URL.Query().Get("action"); v != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", idx))
		args = append(args, v)
		idx++
	}
	if v := r.URL.Query().Get("resource"); v != "" {
		conditions = append(conditions, fmt.Sprintf("resource = $%d", idx))
		args = append(args, v)
		idx++
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			conditions = append(conditions, fmt.Sprintf("timestamp >= $%d", idx))
			args = append(args, t)
			idx++
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			conditions = append(conditions, fmt.Sprintf("timestamp <= $%d", idx))
			args = append(args, t)
			idx++
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// parsePagination extracts page and per_page from query params with defaults.
func parsePagination(r *http.Request) (int, int) {
	page := 1
	perPage := 50

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			perPage = n
		}
	}
	return page, perPage
}

// respondAuditError writes a JSON error response matching the API error contract.
func respondAuditError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message, "message": message})
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
