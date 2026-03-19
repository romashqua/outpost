package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/romashqua/outpost/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// smartRouteAdminReq returns a request with admin claims in the context.
func smartRouteAdminReq(req *http.Request) *http.Request {
	ctx := auth.ContextWithClaims(req.Context(), &auth.TokenClaims{
		UserID:  "00000000-0000-0000-0000-000000000001",
		IsAdmin: true,
	})
	return req.WithContext(ctx)
}

// --- Smart Route: list ---

func TestSmartRouteList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, name, description, is_active, created_at, updated_at\s+FROM smart_routes`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "created_at", "updated_at"}).
			AddRow("11111111-1111-1111-1111-111111111111", "route1", nil, true, now, now).
			AddRow("22222222-2222-2222-2222-222222222222", "route2", ptrStr("desc"), false, now, now))

	r := chi.NewRouter()
	r.Get("/smart-routes", h.listRoutes)
	req := httptest.NewRequest(http.MethodGet, "/smart-routes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var routes []smartRoute
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &routes))
	assert.Len(t, routes, 2)
	assert.Equal(t, "route1", routes[0].Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Smart Route: create ---

func TestSmartRouteCreate_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New().String()

	mock.ExpectQuery(`INSERT INTO smart_routes`).
		WithArgs("test-route", (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "created_at", "updated_at"}).
			AddRow(id, "test-route", nil, true, now, now))

	r := chi.NewRouter()
	r.Post("/smart-routes", h.createRoute)

	body := `{"name":"test-route"}`
	req := httptest.NewRequest(http.MethodPost, "/smart-routes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var sr smartRoute
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sr))
	assert.Equal(t, "test-route", sr.Name)
	assert.True(t, sr.IsActive)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteCreate_DuplicateName(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)

	mock.ExpectQuery(`INSERT INTO smart_routes`).
		WithArgs("dup-route", (*string)(nil)).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	r := chi.NewRouter()
	r.Post("/smart-routes", h.createRoute)

	body := `{"name":"dup-route"}`
	req := httptest.NewRequest(http.MethodPost, "/smart-routes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Smart Route: get ---

func TestSmartRouteGet_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_active, created_at, updated_at\s+FROM smart_routes WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "created_at", "updated_at"}).
			AddRow(id.String(), "route1", nil, true, now, now))

	// Expect entries query
	mock.ExpectQuery(`SELECT e.id, e.smart_route_id, e.entry_type, e.value, e.action, e.proxy_id, p.name, e.priority, e.created_at`).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "smart_route_id", "entry_type", "value", "action", "proxy_id", "proxy_name", "priority", "created_at"}))

	r := chi.NewRouter()
	r.Get("/smart-routes/{id}", h.getRoute)

	req := httptest.NewRequest(http.MethodGet, "/smart-routes/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var sr smartRoute
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sr))
	assert.Equal(t, "route1", sr.Name)
	assert.Empty(t, sr.Entries)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteGet_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_active, created_at, updated_at\s+FROM smart_routes WHERE id = \$1`).
		WithArgs(id).
		WillReturnError(pgx.ErrNoRows)

	r := chi.NewRouter()
	r.Get("/smart-routes/{id}", h.getRoute)

	req := httptest.NewRequest(http.MethodGet, "/smart-routes/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Smart Route: update (partial) ---

func TestSmartRouteUpdate_Partial(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New()

	newName := "updated-name"
	mock.ExpectQuery(`UPDATE smart_routes`).
		WithArgs(id, &newName, (*string)(nil), (*bool)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "description", "is_active", "created_at", "updated_at"}).
			AddRow(id.String(), "updated-name", nil, true, now, now))

	r := chi.NewRouter()
	r.Put("/smart-routes/{id}", h.updateRoute)

	body := `{"name":"updated-name"}`
	req := httptest.NewRequest(http.MethodPut, "/smart-routes/"+id.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var sr smartRoute
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sr))
	assert.Equal(t, "updated-name", sr.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Smart Route: delete ---

func TestSmartRouteDelete_Found(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM smart_routes WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/smart-routes/{id}", h.deleteRoute)

	req := httptest.NewRequest(http.MethodDelete, "/smart-routes/"+id.String(), nil)
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteDelete_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM smart_routes WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	r := chi.NewRouter()
	r.Delete("/smart-routes/{id}", h.deleteRoute)

	req := httptest.NewRequest(http.MethodDelete, "/smart-routes/"+id.String(), nil)
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Smart Route: entries ---

func TestSmartRouteAddEntry(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	routeID := uuid.New()
	entryID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`INSERT INTO smart_route_entries`).
		WithArgs(routeID, "domain", "example.com", "direct", (*string)(nil), 100).
		WillReturnRows(pgxmock.NewRows([]string{"id", "smart_route_id", "entry_type", "value", "action", "proxy_id", "priority", "created_at"}).
			AddRow(entryID.String(), routeID.String(), "domain", "example.com", "direct", nil, 100, now))

	r := chi.NewRouter()
	r.Post("/smart-routes/{id}/entries", h.addEntry)

	body := `{"entry_type":"domain","value":"example.com","action":"direct"}`
	req := httptest.NewRequest(http.MethodPost, "/smart-routes/"+routeID.String()+"/entries", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var entry smartRouteEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	assert.Equal(t, "domain", entry.EntryType)
	assert.Equal(t, "example.com", entry.Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteDeleteEntry(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	routeID := uuid.New()
	entryID := uuid.New()

	mock.ExpectExec(`DELETE FROM smart_route_entries WHERE id = \$1 AND smart_route_id = \$2`).
		WithArgs(entryID, routeID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/smart-routes/{id}/entries/{entryId}", h.deleteEntry)

	req := httptest.NewRequest(http.MethodDelete, "/smart-routes/"+routeID.String()+"/entries/"+entryID.String(), nil)
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Proxy Servers CRUD ---

func TestSmartRouteListProxyServers(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at\s+FROM proxy_servers`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "type", "address", "port", "username", "password", "extra_config", "is_active", "created_at", "updated_at"}).
			AddRow(uuid.New().String(), "proxy1", "socks5", "1.2.3.4", 1080, nil, ptrStr("secret"), nil, true, now, now))

	r := chi.NewRouter()
	r.Get("/proxy-servers", h.listProxyServers)

	req := httptest.NewRequest(http.MethodGet, "/proxy-servers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var servers []proxyServer
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &servers))
	assert.Len(t, servers, 1)
	assert.Equal(t, "proxy1", servers[0].Name)
	assert.Nil(t, servers[0].Password, "password should be stripped")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteCreateProxyServer(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New().String()

	mock.ExpectQuery(`INSERT INTO proxy_servers`).
		WithArgs("myproxy", "socks5", "10.0.0.1", 1080, (*string)(nil), (*string)(nil), (*string)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "type", "address", "port", "username", "password", "extra_config", "is_active", "created_at", "updated_at"}).
			AddRow(id, "myproxy", "socks5", "10.0.0.1", 1080, nil, nil, nil, true, now, now))

	r := chi.NewRouter()
	r.Post("/proxy-servers", h.createProxyServer)

	body := `{"name":"myproxy","type":"socks5","address":"10.0.0.1","port":1080}`
	req := httptest.NewRequest(http.MethodPost, "/proxy-servers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var ps proxyServer
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ps))
	assert.Equal(t, "myproxy", ps.Name)
	assert.Nil(t, ps.Password)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteGetProxyServer(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New()

	mock.ExpectQuery(`SELECT id, name, type, address, port, username, password, extra_config::text, is_active, created_at, updated_at\s+FROM proxy_servers WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "type", "address", "port", "username", "password", "extra_config", "is_active", "created_at", "updated_at"}).
			AddRow(id.String(), "proxy1", "http", "1.2.3.4", 8080, nil, ptrStr("pw"), nil, true, now, now))

	r := chi.NewRouter()
	r.Get("/proxy-servers/{id}", h.getProxyServer)

	req := httptest.NewRequest(http.MethodGet, "/proxy-servers/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var ps proxyServer
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ps))
	assert.Equal(t, "proxy1", ps.Name)
	assert.Nil(t, ps.Password)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteUpdateProxyServer(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	now := time.Now()
	id := uuid.New()

	newName := "renamed"
	mock.ExpectQuery(`UPDATE proxy_servers`).
		WithArgs(id, &newName, (*string)(nil), (*string)(nil), (*int)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*bool)(nil)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "type", "address", "port", "username", "password", "extra_config", "is_active", "created_at", "updated_at"}).
			AddRow(id.String(), "renamed", "socks5", "1.2.3.4", 1080, nil, nil, nil, true, now, now))

	r := chi.NewRouter()
	r.Put("/proxy-servers/{id}", h.updateProxyServer)

	body := `{"name":"renamed"}`
	req := httptest.NewRequest(http.MethodPut, "/proxy-servers/"+id.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var ps proxyServer
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ps))
	assert.Equal(t, "renamed", ps.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteDeleteProxyServer(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	id := uuid.New()

	mock.ExpectExec(`DELETE FROM proxy_servers WHERE id = \$1`).
		WithArgs(id).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/proxy-servers/{id}", h.deleteProxyServer)

	req := httptest.NewRequest(http.MethodDelete, "/proxy-servers/"+id.String(), nil)
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- Network Associations ---

func TestSmartRouteAddNetwork(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	routeID := uuid.New()
	networkID := uuid.New()

	mock.ExpectExec(`INSERT INTO network_smart_routes`).
		WithArgs(networkID.String(), routeID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	r := chi.NewRouter()
	r.Post("/smart-routes/{id}/networks", h.addRouteNetwork)

	body := `{"network_id":"` + networkID.String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/smart-routes/"+routeID.String()+"/networks", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteRemoveNetwork(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	routeID := uuid.New()
	networkID := uuid.New()

	mock.ExpectExec(`DELETE FROM network_smart_routes WHERE network_id = \$1 AND smart_route_id = \$2`).
		WithArgs(networkID, routeID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	r := chi.NewRouter()
	r.Delete("/smart-routes/{id}/networks/{networkId}", h.removeRouteNetwork)

	req := httptest.NewRequest(http.MethodDelete, "/smart-routes/"+routeID.String()+"/networks/"+networkID.String(), nil)
	req = smartRouteAdminReq(req)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSmartRouteListNetworks(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewSmartRouteHandler(mock)
	routeID := uuid.New()
	networkID := uuid.New()

	mock.ExpectQuery(`SELECT nsr.network_id, nsr.smart_route_id, n.name`).
		WithArgs(routeID).
		WillReturnRows(pgxmock.NewRows([]string{"network_id", "smart_route_id", "network_name"}).
			AddRow(networkID.String(), routeID.String(), "MyNetwork"))

	r := chi.NewRouter()
	r.Get("/smart-routes/{id}/networks", h.listRouteNetworks)

	req := httptest.NewRequest(http.MethodGet, "/smart-routes/"+routeID.String()+"/networks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var assocs []networkSmartRoute
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &assocs))
	assert.Len(t, assocs, 1)
	assert.Equal(t, "MyNetwork", assocs[0].NetworkName)
	require.NoError(t, mock.ExpectationsWereMet())
}
