package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
	"github.com/romashqua/outpost/internal/mail"
)

type SettingsHandler struct {
	pool   *pgxpool.Pool
	mailer *mail.Mailer
}

func NewSettingsHandler(pool *pgxpool.Pool, mailer *mail.Mailer) *SettingsHandler {
	return &SettingsHandler{pool: pool, mailer: mailer}
}

func (h *SettingsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.getAll)
	r.With(auth.RequireAdmin).Put("/", h.batchSet)
	r.Get("/{key}", h.get)
	r.With(auth.RequireAdmin).Put("/{key}", h.set)
	r.With(auth.RequireAdmin).Delete("/{key}", h.delete)
	r.With(auth.RequireAdmin).Post("/smtp/test", h.testSMTP)
	return r
}

type settingEntry struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func (h *SettingsHandler) getAll(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query settings")
		return
	}
	defer rows.Close()

	settings := make(map[string]any)
	for rows.Next() {
		var key string
		var value any
		if err := rows.Scan(&key, &value); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan setting")
			return
		}
		settings[key] = value
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate settings")
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

func (h *SettingsHandler) get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var value any
	err := h.pool.QueryRow(r.Context(),
		`SELECT value FROM settings WHERE key = $1`, key,
	).Scan(&value)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "setting not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch setting")
		}
		return
	}

	respondJSON(w, http.StatusOK, settingEntry{Key: key, Value: value})
}

func (h *SettingsHandler) set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var body struct {
		Value any `json:"value"`
	}
	if err := parseBody(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	jsonVal, err := json.Marshal(body.Value)
	if err != nil {
		respondError(w, http.StatusBadRequest, "value is not JSON-serializable")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ($1, $2::jsonb, now())
		 ON CONFLICT (key) DO UPDATE SET value = $2::jsonb, updated_at = now()`,
		key, string(jsonVal),
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}

	respondJSON(w, http.StatusOK, settingEntry{Key: key, Value: body.Value})
}

// batchSet accepts a JSON object of key-value pairs and upserts them all.
func (h *SettingsHandler) batchSet(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := parseBody(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(body) == 0 {
		respondError(w, http.StatusBadRequest, "request body must be a non-empty JSON object")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback(r.Context())

	for key, value := range body {
		jsonVal, err := json.Marshal(value)
		if err != nil {
			respondError(w, http.StatusBadRequest, "value for "+key+" is not JSON-serializable")
			return
		}
		_, err = tx.Exec(r.Context(),
			`INSERT INTO settings (key, value, updated_at)
			 VALUES ($1, $2::jsonb, now())
			 ON CONFLICT (key) DO UPDATE SET value = $2::jsonb, updated_at = now()`,
			key, string(jsonVal),
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to save setting: "+key)
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to commit settings")
		return
	}

	respondJSON(w, http.StatusOK, body)
}

func (h *SettingsHandler) delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM settings WHERE key = $1`, key)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete setting")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "setting not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// testSMTP sends a test email to verify SMTP configuration.
func (h *SettingsHandler) testSMTP(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		respondError(w, http.StatusBadRequest, "SMTP is not configured")
		return
	}

	var body struct {
		To string `json:"to"`
	}
	if err := parseBody(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.To == "" {
		respondError(w, http.StatusBadRequest, "to (email address) is required")
		return
	}

	err := h.mailer.Send(context.Background(), body.To,
		"Outpost VPN - SMTP Test",
		"<h1>SMTP Configuration Test</h1><p>If you are reading this, your SMTP settings are working correctly.</p>")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to send test email: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
