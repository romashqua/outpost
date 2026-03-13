package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingsHandler struct {
	pool *pgxpool.Pool
}

func NewSettingsHandler(pool *pgxpool.Pool) *SettingsHandler {
	return &SettingsHandler{pool: pool}
}

func (h *SettingsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.getAll)
	r.Get("/{key}", h.get)
	r.Put("/{key}", h.set)
	r.Delete("/{key}", h.delete)
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
			continue
		}
		settings[key] = value
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
		respondError(w, http.StatusNotFound, "setting not found")
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

	_, err := h.pool.Exec(r.Context(),
		`INSERT INTO settings (key, value, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`,
		key, body.Value,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save setting")
		return
	}

	respondJSON(w, http.StatusOK, settingEntry{Key: key, Value: body.Value})
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
