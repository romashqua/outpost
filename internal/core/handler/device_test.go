package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/romashqua/outpost/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deviceTestJWTSecret = "test-secret-key-for-device-tests"

// makeToken generates a signed JWT for testing.
func makeToken(t *testing.T, userID string, isAdmin bool) string {
	t.Helper()
	tok, err := auth.GenerateToken(deviceTestJWTSecret, auth.TokenClaims{
		UserID:   userID,
		Username: "testuser",
		Email:    "test@example.com",
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	require.NoError(t, err)
	return tok
}

// setupDeviceRouter creates a chi router with JWT middleware + DeviceHandler routes.
func setupDeviceRouter(t *testing.T, mock pgxmock.PgxPoolIface) *chi.Mux {
	t.Helper()
	h := NewDeviceHandler(mock)
	r := chi.NewRouter()
	r.Use(auth.JWTMiddleware(deviceTestJWTSecret))
	r.Mount("/devices", h.Routes())
	return r
}

// --- list ---

func TestDeviceList_Admin(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New().String()
	deviceID := uuid.New()
	userID := uuid.New()
	now := time.Now().Truncate(time.Second)
	netID := uuid.New()
	netName := "office"

	// COUNT query
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM devices`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

	// SELECT query
	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(50, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "network_id", "network_name",
			"created_at", "updated_at",
		}).AddRow(
			deviceID, userID, "alice", "laptop", "pubkey123",
			"10.0.0.2", true, nil, &netID, &netName, now, now,
		))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID, true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total"])
	devices := body["devices"].([]any)
	assert.Len(t, devices, 1)
	assert.Equal(t, "laptop", devices[0].(map[string]any)["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceList_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New().String()
	router := setupDeviceRouter(t, mock)

	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID, false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// RequireAdmin middleware returns 403 for non-admin
	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- listMy ---

func TestDeviceListMy(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()
	now := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM devices WHERE user_id`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(userID, 50, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "network_id", "network_name",
			"created_at", "updated_at",
		}).AddRow(
			deviceID, userID, "bob", "phone", "pk1",
			"10.0.0.3", true, nil, nil, nil, now, now,
		))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices/my", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["total"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- create ---

func TestDeviceCreate_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()
	netID := uuid.New()
	now := time.Now().Truncate(time.Second)

	// Network lookup (no network_id in request, picks first active)
	mock.ExpectQuery(`SELECT id, COALESCE\(tunnel_cidr, address\)`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "alloc_cidr"}).
			AddRow(netID, "10.0.0.0/24"))

	// INSERT with IP allocation CTE — args include dynamically generated keys
	mock.ExpectQuery(`INSERT INTO devices`).
		WithArgs(userID, "my-laptop", pgxmock.AnyArg(), pgxmock.AnyArg(), "10.0.0.0/24", netID).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "created_at", "updated_at",
		}).AddRow(
			deviceID, userID, "my-laptop", "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=",
			"10.0.0.2", false, nil, now, now,
		))

	router := setupDeviceRouter(t, mock)
	body := `{"name":"my-laptop"}`
	req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "my-laptop", resp["name"])
	assert.Equal(t, "10.0.0.2", resp["assigned_ip"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceCreate_MissingName(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	router := setupDeviceRouter(t, mock)
	body := `{"wireguard_pubkey":"something"}`
	req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, uuid.New().String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "name is required")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceCreate_NonAdminCannotCreateForOthers(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	callerID := uuid.New().String()
	otherUserID := uuid.New().String()

	router := setupDeviceRouter(t, mock)
	body := fmt.Sprintf(`{"name":"laptop","user_id":"%s"}`, otherUserID)
	req := httptest.NewRequest(http.MethodPost, "/devices", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, callerID, false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- get ---

func TestDeviceGet_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()
	now := time.Now().Truncate(time.Second)

	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(deviceID, userID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "created_at", "updated_at",
		}).AddRow(
			deviceID, userID, "alice", "laptop", "pk1",
			"10.0.0.2", true, nil, now, now,
		))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "laptop", resp["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceGet_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()

	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(deviceID, userID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "created_at", "updated_at",
		})) // no rows

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceGet_IDOR_NonAdminCannotSeeOthers(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	callerID := uuid.New()
	deviceID := uuid.New()

	// The SQL has (user_id = $2 OR $3 = true), non-admin so $3=false.
	// Device belongs to someone else — query returns no rows.
	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(deviceID, callerID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "created_at", "updated_at",
		})) // empty — IDOR blocked

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, callerID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceGet_AdminSeesAll(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	ownerID := uuid.New()
	deviceID := uuid.New()
	now := time.Now().Truncate(time.Second)

	// Admin: $3 = true, so ownership check is bypassed.
	mock.ExpectQuery(`SELECT d\.id, d\.user_id`).
		WithArgs(deviceID, adminID.String(), true).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "owner_name", "name", "wireguard_pubkey",
			"assigned_ip", "is_approved", "last_handshake", "created_at", "updated_at",
		}).AddRow(
			deviceID, ownerID, "other", "their-laptop", "pk2",
			"10.0.0.5", true, nil, now, now,
		))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "their-laptop", resp["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- update ---

func TestDeviceUpdate_NameChange(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()
	now := time.Now().Truncate(time.Second)
	netID := uuid.New()
	newName := "new-laptop-name"

	mock.ExpectQuery(`UPDATE devices SET name`).
		WithArgs(&newName, deviceID, userID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "name", "wireguard_pubkey", "assigned_ip",
			"is_approved", "last_handshake", "network_id", "created_at", "updated_at",
		}).AddRow(
			deviceID, userID, newName, "pk1", "10.0.0.2",
			true, nil, &netID, now, now,
		))

	router := setupDeviceRouter(t, mock)
	body := `{"name":"new-laptop-name"}`
	req := httptest.NewRequest(http.MethodPut, "/devices/"+deviceID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "new-laptop-name", resp["name"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceUpdate_IDOR(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	callerID := uuid.New()
	deviceID := uuid.New()
	newName := "hacked"

	// Non-admin update on someone else's device — no rows returned.
	mock.ExpectQuery(`UPDATE devices SET name`).
		WithArgs(&newName, deviceID, callerID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "user_id", "name", "wireguard_pubkey", "assigned_ip",
			"is_approved", "last_handshake", "network_id", "created_at", "updated_at",
		})) // empty

	router := setupDeviceRouter(t, mock)
	body := `{"name":"hacked"}`
	req := httptest.NewRequest(http.MethodPut, "/devices/"+deviceID.String(), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+makeToken(t, callerID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- delete ---

func TestDeviceDelete_AdminOK(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	deviceID := uuid.New()
	ownerID := uuid.New()

	// Fetch device owner + pubkey with ownership check
	mock.ExpectQuery(`SELECT user_id::text, wireguard_pubkey FROM devices WHERE id`).
		WithArgs(deviceID, adminID.String(), true).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "wireguard_pubkey"}).
			AddRow(ownerID.String(), "some-pubkey"))

	// DELETE
	mock.ExpectExec(`DELETE FROM devices WHERE id`).
		WithArgs(deviceID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodDelete, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceDelete_IDOR(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	callerID := uuid.New()
	deviceID := uuid.New()

	// Non-admin tries to delete someone else's device — no rows.
	mock.ExpectQuery(`SELECT user_id::text, wireguard_pubkey FROM devices WHERE id`).
		WithArgs(deviceID, callerID.String(), false).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "wireguard_pubkey"})) // empty

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodDelete, "/devices/"+deviceID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, callerID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- approve ---

func TestDeviceApprove_AdminOK(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	deviceID := uuid.New()

	mock.ExpectQuery(`UPDATE devices SET is_approved = true`).
		WithArgs(deviceID).
		WillReturnRows(pgxmock.NewRows([]string{"wireguard_pubkey", "assigned_ip"}).
			AddRow("pubkey123", "10.0.0.2/32"))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "approved", resp["status"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceApprove_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceApprove_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	deviceID := uuid.New()

	mock.ExpectQuery(`UPDATE devices SET is_approved = true`).
		WithArgs(deviceID).
		WillReturnRows(pgxmock.NewRows([]string{"wireguard_pubkey", "assigned_ip"})) // empty

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- revoke ---

func TestDeviceRevoke_AdminOK(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	deviceID := uuid.New()

	mock.ExpectQuery(`UPDATE devices SET is_approved = false`).
		WithArgs(deviceID).
		WillReturnRows(pgxmock.NewRows([]string{"wireguard_pubkey"}).
			AddRow("pubkey123"))

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "revoked", resp["status"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRevoke_NonAdminForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	userID := uuid.New()
	deviceID := uuid.New()

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, userID.String(), false))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeviceRevoke_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	adminID := uuid.New()
	deviceID := uuid.New()

	mock.ExpectQuery(`UPDATE devices SET is_approved = false`).
		WithArgs(deviceID).
		WillReturnRows(pgxmock.NewRows([]string{"wireguard_pubkey"})) // empty

	router := setupDeviceRouter(t, mock)
	req := httptest.NewRequest(http.MethodPost, "/devices/"+deviceID.String()+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, adminID.String(), true))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
