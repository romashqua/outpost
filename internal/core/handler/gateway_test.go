package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/romashqua/outpost/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGatewayRouter creates a chi router with JWT middleware + GatewayHandler routes.
func setupGatewayRouter(t *testing.T, mock pgxmock.PgxPoolIface) *chi.Mux {
	t.Helper()
	h := NewGatewayHandler(mock)
	r := chi.NewRouter()
	r.Use(auth.JWTMiddleware(deviceTestJWTSecret))
	r.Mount("/gateways", h.Routes())
	return r
}

// --- list ---

func TestGatewayList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()
	netID := uuid.New()
	now := time.Now().Truncate(time.Second)
	pubIP := "1.2.3.4"

	// COUNT
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM gateways`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

	// SELECT gateways
	mock.ExpectQuery(`SELECT id, network_id, name`).
		WithArgs(50, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		}).AddRow(
			gwID, &netID, "gw-1", &pubIP, "wgpubkey",
			"gw.example.com:51820", true, "healthy", 10, nil, now, now,
		))

	// loadGatewayNetworks
	mock.ExpectQuery(`SELECT gn\.gateway_id, n\.id, n\.name, n\.address`).
		WithArgs([]uuid.UUID{gwID}).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id", "id", "name", "address"}).
			AddRow(gwID, netID, "office", "10.0.0.0/24"))

	router := setupGatewayRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/gateways", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total"])
	gateways := body["gateways"].([]any)
	assert.Len(t, gateways, 1)
	gw := gateways[0].(map[string]any)
	assert.Equal(t, "gw-1", gw["name"])
	networks := gw["networks"].([]any)
	assert.Len(t, networks, 1)
	assert.Equal(t, "office", networks[0].(map[string]any)["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- create ---

func TestGatewayCreate_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()
	netID := uuid.New()
	now := time.Now().Truncate(time.Second)
	pubIP := "5.6.7.8"

	// Begin transaction
	mock.ExpectBegin()

	// INSERT gateway — args include dynamically generated keys/token
	mock.ExpectQuery(`INSERT INTO gateways`).
		WithArgs(netID, "gw-new", pgxmock.AnyArg(), pgxmock.AnyArg(), "gw.example.com:51820", pgxmock.AnyArg(), &pubIP, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		}).AddRow(
			gwID, &netID, "gw-new", &pubIP, "wgpubkey",
			"gw.example.com:51820", false, "unknown", 0, nil, now, now,
		))

	// INSERT gateway_networks
	mock.ExpectExec(`INSERT INTO gateway_networks`).
		WithArgs(gwID, netID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	// Commit
	mock.ExpectCommit()

	// loadGatewayNetworks after commit
	mock.ExpectQuery(`SELECT gn\.gateway_id, n\.id, n\.name, n\.address`).
		WithArgs([]uuid.UUID{gwID}).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id", "id", "name", "address"}).
			AddRow(gwID, netID, "office", "10.0.0.0/24"))

	router := setupGatewayRouter(t, mock)
	body := `{"name":"gw-new","endpoint":"gw.example.com","public_ip":"5.6.7.8","network_ids":["` + netID.String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "gw-new", resp["name"])
	assert.NotEmpty(t, resp["token"])
	assert.NotEmpty(t, resp["private_key"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayCreate_MissingName(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupGatewayRouter(t, mock)
	body := `{"endpoint":"gw.example.com","public_ip":"1.2.3.4","network_ids":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "name is required")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayCreate_MissingPublicIP(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupGatewayRouter(t, mock)
	body := `{"name":"gw-1","endpoint":"gw.example.com","network_ids":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "public_ip is required")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayCreate_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupGatewayRouter(t, mock)
	body := `{"name":"gw-1","endpoint":"gw.example.com:51820","public_ip":"1.2.3.4","network_ids":["` + uuid.New().String() + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/gateways", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- get ---

func TestGatewayGet_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	gwID := uuid.New()
	netID := uuid.New()
	now := time.Now().Truncate(time.Second)
	pubIP := "1.2.3.4"

	mock.ExpectQuery(`SELECT id, network_id, name`).
		WithArgs(gwID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		}).AddRow(
			gwID, &netID, "gw-1", &pubIP, "wgpubkey",
			"gw.example.com:51820", true, "healthy", 10, nil, now, now,
		))

	// loadGatewayNetworks
	mock.ExpectQuery(`SELECT gn\.gateway_id, n\.id, n\.name, n\.address`).
		WithArgs([]uuid.UUID{gwID}).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id", "id", "name", "address"}).
			AddRow(gwID, netID, "office", "10.0.0.0/24"))

	router := setupGatewayRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/gateways/"+gwID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "gw-1", resp["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayGet_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	gwID := uuid.New()

	mock.ExpectQuery(`SELECT id, network_id, name`).
		WithArgs(gwID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		})) // empty

	router := setupGatewayRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/gateways/"+gwID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- update ---

func TestGatewayUpdate_PartialUpdate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()
	netID := uuid.New()
	now := time.Now().Truncate(time.Second)
	pubIP := "1.2.3.4"
	newName := "gw-renamed"

	mock.ExpectBegin()

	// UPDATE: args are (id, name, endpoint, public_ip, priority, is_active)
	mock.ExpectQuery(`UPDATE gateways`).
		WithArgs(gwID, &newName, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		}).AddRow(
			gwID, &netID, newName, &pubIP, "wgpubkey",
			"gw.example.com:51820", true, "healthy", 10, nil, now, now,
		))

	mock.ExpectCommit()

	// loadGatewayNetworks
	mock.ExpectQuery(`SELECT gn\.gateway_id, n\.id, n\.name, n\.address`).
		WithArgs([]uuid.UUID{gwID}).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id", "id", "name", "address"}).
			AddRow(gwID, netID, "office", "10.0.0.0/24"))

	router := setupGatewayRouter(t, mock)
	body := `{"name":"gw-renamed"}`
	req := httptest.NewRequest(http.MethodPut, "/gateways/"+gwID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "gw-renamed", resp["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayUpdate_NetworkIDs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()
	netID1 := uuid.New()
	netID2 := uuid.New()
	now := time.Now().Truncate(time.Second)
	pubIP := "1.2.3.4"

	mock.ExpectBegin()

	// UPDATE: args are (id, name, endpoint, public_ip, priority, is_active)
	mock.ExpectQuery(`UPDATE gateways`).
		WithArgs(gwID, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "network_id", "name", "public_ip", "wireguard_pubkey",
			"endpoint", "is_active", "health_status", "priority", "last_seen", "created_at", "updated_at",
		}).AddRow(
			gwID, &netID1, "gw-1", &pubIP, "wgpubkey",
			"gw.example.com:51820", true, "healthy", 10, nil, now, now,
		))

	// Delete old networks
	mock.ExpectExec(`DELETE FROM gateway_networks WHERE gateway_id`).
		WithArgs(gwID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	// Insert new networks
	mock.ExpectExec(`INSERT INTO gateway_networks`).
		WithArgs(gwID, netID1).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`INSERT INTO gateway_networks`).
		WithArgs(gwID, netID2).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	// Update legacy network_id
	mock.ExpectExec(`UPDATE gateways SET network_id`).
		WithArgs(gwID, netID1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	mock.ExpectCommit()

	// loadGatewayNetworks
	mock.ExpectQuery(`SELECT gn\.gateway_id, n\.id, n\.name, n\.address`).
		WithArgs([]uuid.UUID{gwID}).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id", "id", "name", "address"}).
			AddRow(gwID, netID1, "net-1", "10.0.0.0/24").
			AddRow(gwID, netID2, "net-2", "10.1.0.0/24"))

	router := setupGatewayRouter(t, mock)
	body := `{"network_ids":["` + netID1.String() + `","` + netID2.String() + `"]}`
	req := httptest.NewRequest(http.MethodPut, "/gateways/"+gwID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	networks := resp["networks"].([]any)
	assert.Len(t, networks, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayUpdate_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupGatewayRouter(t, mock)
	gwID := uuid.New()
	body := `{"name":"renamed"}`
	req := httptest.NewRequest(http.MethodPut, "/gateways/"+gwID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- delete ---

func TestGatewayDelete_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()

	mock.ExpectExec(`DELETE FROM gateways WHERE id`).
		WithArgs(gwID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	router := setupGatewayRouter(t, mock)
	req := httptest.NewRequest(http.MethodDelete, "/gateways/"+gwID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayDelete_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	gwID := uuid.New()

	mock.ExpectExec(`DELETE FROM gateways WHERE id`).
		WithArgs(gwID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	router := setupGatewayRouter(t, mock)
	req := httptest.NewRequest(http.MethodDelete, "/gateways/"+gwID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGatewayDelete_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupGatewayRouter(t, mock)
	gwID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/gateways/"+gwID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
