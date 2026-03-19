package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockS2SNotifier struct{ called bool }

func (m *mockS2SNotifier) NotifyS2SUpdate(gatewayID, tunnelID, action string) { m.called = true }

// ---- List ----

func TestS2SList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	now := time.Now()
	id1 := uuid.New().String()
	id2 := uuid.New().String()
	hubGW := "gw-1"

	mock.ExpectQuery(`SELECT id, name, COALESCE\(description, ''\), topology, hub_gateway_id, is_active, created_at, updated_at`).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "topology", "hub_gateway_id", "is_active", "created_at", "updated_at"}).
				AddRow(id1, "mesh-tunnel", "", "mesh", nil, true, now, now).
				AddRow(id2, "hub-spoke-tunnel", "office", "hub_spoke", &hubGW, true, now, now),
		)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.list(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var tunnels []s2sTunnel
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tunnels))
	assert.Len(t, tunnels, 2)
	assert.Equal(t, "mesh", tunnels[0].Topology)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Create ----

func TestS2SCreateMesh(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	now := time.Now()
	id := uuid.New().String()

	mock.ExpectQuery(`INSERT INTO s2s_tunnels`).
		WithArgs("dc-mesh", "datacenter mesh", "mesh", (*string)(nil)).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "topology", "hub_gateway_id", "is_active", "created_at", "updated_at"}).
				AddRow(id, "dc-mesh", "datacenter mesh", "mesh", nil, true, now, now),
		)

	body := `{"name":"dc-mesh","description":"datacenter mesh","topology":"mesh"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var tun s2sTunnel
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tun))
	assert.Equal(t, "mesh", tun.Topology)
	assert.Equal(t, "dc-mesh", tun.Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SCreateHubSpokeWithoutHub(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	body := `{"name":"office-tunnel","topology":"hub_spoke"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp["message"], "hub_gateway_id is required")

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SCreateDuplicateName(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	mock.ExpectQuery(`INSERT INTO s2s_tunnels`).
		WithArgs("dc-mesh", "", "mesh", (*string)(nil)).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "s2s_tunnels_name_key"})

	body := `{"name":"dc-mesh","topology":"mesh"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.create(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp["message"], "name already exists")

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Get ----

func TestS2SGetFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	tunnelID := uuid.New()
	now := time.Now()

	mock.ExpectQuery(`SELECT id, name, COALESCE\(description, ''\), topology, hub_gateway_id, is_active, created_at, updated_at\s+FROM s2s_tunnels WHERE id = \$1`).
		WithArgs(tunnelID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "topology", "hub_gateway_id", "is_active", "created_at", "updated_at"}).
				AddRow(tunnelID.String(), "mesh-1", "test", "mesh", nil, true, now, now),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+tunnelID.String(), nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	w := httptest.NewRecorder()

	h.get(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var tun s2sTunnel
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tun))
	assert.Equal(t, "mesh-1", tun.Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SGetNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	tunnelID := uuid.New()

	mock.ExpectQuery(`SELECT id, name, COALESCE\(description, ''\), topology, hub_gateway_id, is_active, created_at, updated_at\s+FROM s2s_tunnels WHERE id = \$1`).
		WithArgs(tunnelID).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/"+tunnelID.String(), nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	w := httptest.NewRecorder()

	h.get(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Update ----

func TestS2SUpdate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)

	tunnelID := uuid.New()
	now := time.Now()
	newName := "updated-mesh"
	newDesc := "updated desc"

	mock.ExpectQuery(`UPDATE s2s_tunnels SET`).
		WithArgs(&newName, &newDesc, tunnelID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "topology", "hub_gateway_id", "is_active", "created_at", "updated_at"}).
				AddRow(tunnelID.String(), newName, newDesc, "mesh", nil, true, now, now),
		)

	body := fmt.Sprintf(`{"name":"%s","description":"%s"}`, newName, newDesc)
	req := httptest.NewRequest(http.MethodPut, "/"+tunnelID.String(), bytes.NewBufferString(body))
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var tun s2sTunnel
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tun))
	assert.Equal(t, "updated-mesh", tun.Name)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Delete ----

func TestS2SDeleteFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()

	mock.ExpectExec(`DELETE FROM s2s_tunnels WHERE id = \$1`).
		WithArgs(tunnelID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/"+tunnelID.String(), nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SDeleteNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()

	mock.ExpectExec(`DELETE FROM s2s_tunnels WHERE id = \$1`).
		WithArgs(tunnelID).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	req := httptest.NewRequest(http.MethodDelete, "/"+tunnelID.String(), nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Members ----

func TestS2SAddMember(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	gatewayID := uuid.New()

	mock.ExpectExec(`INSERT INTO s2s_tunnel_members`).
		WithArgs(tunnelID, gatewayID, []string{"192.168.1.0/24"}).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	body := fmt.Sprintf(`{"gateway_id":"%s","local_subnets":["192.168.1.0/24"]}`, gatewayID)
	req := httptest.NewRequest(http.MethodPost, "/"+tunnelID.String()+"/members", bytes.NewBufferString(body))
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.addMember(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.True(t, notifier.called)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SRemoveMember(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	gatewayID := uuid.New()

	mock.ExpectExec(`DELETE FROM s2s_tunnel_members WHERE tunnel_id = \$1 AND gateway_id = \$2`).
		WithArgs(tunnelID, gatewayID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String(), "gatewayId": gatewayID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.removeMember(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, notifier.called)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SListMembers(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	gw1 := uuid.New().String()

	mock.ExpectQuery(`SELECT m\.tunnel_id, m\.gateway_id, g\.name`).
		WithArgs(tunnelID).
		WillReturnRows(
			pgxmock.NewRows([]string{"tunnel_id", "gateway_id", "name", "local_subnets"}).
				AddRow(tunnelID.String(), gw1, "gw-east", []string{"10.1.0.0/16"}),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+tunnelID.String()+"/members", nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	w := httptest.NewRecorder()

	h.listMembers(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var members []s2sMember
	require.NoError(t, json.NewDecoder(w.Body).Decode(&members))
	assert.Len(t, members, 1)
	assert.Equal(t, "gw-east", members[0].GatewayName)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Routes ----

func TestS2SAddRoute(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	gatewayID := uuid.New()
	routeID := uuid.New().String()

	mock.ExpectQuery(`INSERT INTO s2s_routes`).
		WithArgs(tunnelID, "10.20.0.0/16", gatewayID, 100).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "tunnel_id", "destination", "via_gateway", "gateway_name", "metric", "is_active", "created_at"}).
				AddRow(routeID, tunnelID.String(), "10.20.0.0/16", gatewayID.String(), "gw-west", 100, true, "2025-01-01T00:00:00Z"),
		)

	// notifyTunnelMembers will query for members
	mock.ExpectQuery(`SELECT gateway_id::text FROM s2s_tunnel_members WHERE tunnel_id = \$1`).
		WithArgs(tunnelID.String()).
		WillReturnRows(
			pgxmock.NewRows([]string{"gateway_id"}).
				AddRow(gatewayID.String()),
		)

	body := fmt.Sprintf(`{"destination":"10.20.0.0/16","via_gateway":"%s"}`, gatewayID)
	req := httptest.NewRequest(http.MethodPost, "/"+tunnelID.String()+"/routes", bytes.NewBufferString(body))
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.addRoute(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var rt s2sRoute
	require.NoError(t, json.NewDecoder(w.Body).Decode(&rt))
	assert.Equal(t, "10.20.0.0/16", rt.Destination)
	assert.Equal(t, 100, rt.Metric)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SRemoveRoute(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	routeID := uuid.New()

	mock.ExpectExec(`DELETE FROM s2s_routes WHERE id = \$1 AND tunnel_id = \$2`).
		WithArgs(routeID, tunnelID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	// notifyTunnelMembers query
	mock.ExpectQuery(`SELECT gateway_id::text FROM s2s_tunnel_members WHERE tunnel_id = \$1`).
		WithArgs(tunnelID.String()).
		WillReturnRows(pgxmock.NewRows([]string{"gateway_id"}))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String(), "routeId": routeID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.removeRoute(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestS2SListRoutes(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	notifier := &mockS2SNotifier{}
	h := NewS2SHandler(mock, notifier)
	tunnelID := uuid.New()
	routeID := uuid.New().String()
	gwID := uuid.New().String()

	mock.ExpectQuery(`SELECT sr\.id, sr\.tunnel_id, sr\.destination::text`).
		WithArgs(tunnelID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "tunnel_id", "destination", "via_gateway", "name", "metric", "is_active", "created_at"}).
				AddRow(routeID, tunnelID.String(), "10.30.0.0/16", gwID, "gw-south", 50, true, "2025-01-01T00:00:00Z"),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+tunnelID.String()+"/routes", nil)
	req = withChiParams(req, map[string]string{"id": tunnelID.String()})
	w := httptest.NewRecorder()

	h.listRoutes(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var routes []s2sRoute
	require.NoError(t, json.NewDecoder(w.Body).Decode(&routes))
	assert.Len(t, routes, 1)
	assert.Equal(t, "10.30.0.0/16", routes[0].Destination)
	assert.Equal(t, 50, routes[0].Metric)

	require.NoError(t, mock.ExpectationsWereMet())
}
