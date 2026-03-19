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

func TestDashboardStats_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	rows := mock.NewRows([]string{
		"active_users", "total_users",
		"active_devices", "total_devices",
		"active_gateways", "total_gateways",
		"active_networks", "s2s_tunnels",
	}).AddRow(5, 10, 8, 20, 2, 3, 4, 1)

	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	h := NewDashboardHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body dashboardStats
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, 5, body.ActiveUsers)
	assert.Equal(t, 10, body.TotalUsers)
	assert.Equal(t, 8, body.ActiveDevices)
	assert.Equal(t, 20, body.TotalDevices)
	assert.Equal(t, 2, body.ActiveGateways)
	assert.Equal(t, 3, body.TotalGateways)
	assert.Equal(t, 4, body.ActiveNetworks)
	assert.Equal(t, 1, body.S2STunnels)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDashboardStats_QueryError_Returns500(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("db down"))

	h := NewDashboardHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Contains(t, body["error"], "dashboard stats")
	assert.NoError(t, mock.ExpectationsWereMet())
}
