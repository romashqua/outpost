package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romashqua/outpost/internal/auth"
)

// withClaims returns a copy of r with the given auth claims injected into the context.
func withClaims(r *http.Request, claims *auth.TokenClaims) *http.Request {
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}

func notifAdminClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:   "admin-uuid",
		Username: "admin",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}
}

func notifUserClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:   "user-uuid",
		Username: "alice",
		Email:    "alice@example.com",
		IsAdmin:  false,
	}
}

func TestNotificationList_Unauthorized(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewNotificationHandler(mock)
	router := h.Routes()

	// No claims in context.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestNotificationList_Admin_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	userID := "admin-uuid"
	rows := mock.NewRows([]string{"id", "timestamp", "action", "resource", "details", "user_id"}).
		AddRow(int64(1), now, "CREATE", "device", []byte(`{"name":"dev1"}`), &userID).
		AddRow(int64(2), now.Add(-time.Minute), "DELETE", "user", nil, (*string)(nil))

	mock.ExpectQuery(`SELECT id, timestamp, action, resource, details, user_id::text\s+FROM audit_log`).
		WithArgs(30).
		WillReturnRows(rows)

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withClaims(req, notifAdminClaims())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(2), body["total"])

	items, ok := body["notifications"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)

	first := items[0].(map[string]any)
	assert.Equal(t, "CREATE", first["action"])
	assert.Equal(t, "device", first["resource"])
	assert.NotNil(t, first["details"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationList_RegularUser_FiltersOnUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)
	userID := "user-uuid"
	rows := mock.NewRows([]string{"id", "timestamp", "action", "resource", "details", "user_id"}).
		AddRow(int64(10), now, "login", "session", nil, &userID)

	mock.ExpectQuery(`SELECT id, timestamp, action, resource, details, user_id::text\s+FROM audit_log\s+WHERE user_id`).
		WithArgs("user-uuid", 30).
		WillReturnRows(rows)

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = withClaims(req, notifUserClaims())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(1), body["total"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationList_CustomLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	rows := mock.NewRows([]string{"id", "timestamp", "action", "resource", "details", "user_id"})
	mock.ExpectQuery(`SELECT`).WithArgs(5).WillReturnRows(rows)

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/?limit=5", nil)
	req = withClaims(req, notifAdminClaims())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationUnreadCount_Admin(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	rows := mock.NewRows([]string{"count"}).AddRow(7)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_log WHERE timestamp`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnRows(rows)

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/unread-count", nil)
	req = withClaims(req, notifAdminClaims())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(7), body["count"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationUnreadCount_RegularUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	rows := mock.NewRows([]string{"count"}).AddRow(3)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_log WHERE user_id`).
		WithArgs("user-uuid", pgxmock.AnyArg()).
		WillReturnRows(rows)

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodGet, "/unread-count", nil)
	req = withClaims(req, notifUserClaims())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(3), body["count"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNotificationMarkRead_Returns200(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewNotificationHandler(mock)
	router := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/mark-read", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}
