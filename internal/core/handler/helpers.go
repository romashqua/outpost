package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	defaultPage    = 1
	defaultPerPage = 50
	maxPerPage     = 100
)

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondError writes a JSON error response with the given status code and message.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message, "message": message})
}

// parseBody decodes a JSON request body into dst. Returns an error suitable
// for showing to the client if decoding fails.
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

// parseUUID extracts a chi URL parameter by name and parses it as a UUID.
func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	raw := chi.URLParam(r, param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID %q: %w", raw, err)
	}
	return id, nil
}

// parsePagination extracts page and per_page query parameters from the request.
// It returns safe defaults when values are missing or invalid.
func parsePagination(r *http.Request) (page, perPage int) {
	page = queryInt(r, "page", defaultPage)
	perPage = queryInt(r, "per_page", defaultPerPage)

	if page < 1 {
		page = defaultPage
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

// queryInt reads an integer query parameter, returning fallback on missing or
// invalid values.
func queryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
