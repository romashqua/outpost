package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthz_ReturnsOK(t *testing.T) {
	h := NewHealthHandler(nil)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

func TestReadyz_NilPool_Returns503(t *testing.T) {
	h := NewHealthHandler(nil)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Contains(t, body["error"], "not configured")
}

func TestReadyz_PingSuccess_Returns200(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectPing()

	h := NewHealthHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReadyz_PingFails_Returns503(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectPing().WillReturnError(fmt.Errorf("connection refused"))

	h := NewHealthHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Contains(t, body["error"], "unreachable")
	assert.NoError(t, mock.ExpectationsWereMet())
}
