package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestRespondJSON_Nil(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body for nil data, got %q", w.Body.String())
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	body := w.Body.String()
	if !bytes.Contains(w.Body.Bytes(), []byte("something went wrong")) {
		t.Errorf("expected error message in body, got %q", body)
	}
}

func TestParseBody(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"valid", `{"name":"test"}`, false},
		{"empty body", "", true},
		{"invalid json", `{bad`, true},
		{"unknown field", `{"name":"test","unknown":"field"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst struct {
				Name string `json:"name"`
			}
			var r *http.Request
			if tt.body == "" {
				r = httptest.NewRequest("POST", "/", nil)
				r.Body = nil
			} else {
				r = httptest.NewRequest("POST", "/", bytes.NewBufferString(tt.body))
			}
			err := parseBody(r, &dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseBody() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePagination(t *testing.T) {
	tests := []struct {
		query       string
		wantPage    int
		wantPerPage int
	}{
		{"", 1, 50},
		{"?page=2&per_page=25", 2, 25},
		{"?page=-1&per_page=0", 1, 50},
		{"?page=1&per_page=999", 1, 100},
		{"?page=abc", 1, 50},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/test"+tt.query, nil)
			page, perPage := parsePagination(r)
			if page != tt.wantPage {
				t.Errorf("page = %d, want %d", page, tt.wantPage)
			}
			if perPage != tt.wantPerPage {
				t.Errorf("perPage = %d, want %d", perPage, tt.wantPerPage)
			}
		})
	}
}
