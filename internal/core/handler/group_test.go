package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withChiParams injects chi URL params into the request context.
func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// ---- List ----

func TestGroupList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)

	now := time.Now()
	id1 := uuid.New()
	id2 := uuid.New()

	mock.ExpectQuery(`SELECT g\.id, g\.name, g\.description, g\.is_system, g\.created_at`).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "is_system", "created_at", "member_count"}).
				AddRow(id1, "admins", "Admin group", true, now, 3).
				AddRow(id2, "devs", "Developers", false, now, 1),
		)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.list(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var groups []groupListItem
	require.NoError(t, json.NewDecoder(w.Body).Decode(&groups))
	assert.Len(t, groups, 2)
	assert.Equal(t, "admins", groups[0].Name)
	assert.Equal(t, 3, groups[0].MemberCount)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Create ----

func TestGroupCreate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)

	now := time.Now()
	id := uuid.New()

	mock.ExpectQuery(`INSERT INTO groups`).
		WithArgs("engineers", "Engineering team").
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "is_system", "created_at"}).
				AddRow(id, "engineers", "Engineering team", false, now),
		)

	body := `{"name":"engineers","description":"Engineering team"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var g groupListItem
	require.NoError(t, json.NewDecoder(w.Body).Decode(&g))
	assert.Equal(t, "engineers", g.Name)
	assert.Equal(t, 0, g.MemberCount)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupCreateDuplicate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)

	mock.ExpectQuery(`INSERT INTO groups`).
		WithArgs("engineers", "").
		WillReturnError(&pgconn.PgError{Code: "23505"})

	body := `{"name":"engineers"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.create(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Contains(t, resp["message"], "already exists")

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Get ----

func TestGroupGetFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)

	groupID := uuid.New()
	now := time.Now()
	userID := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_system, created_at\s+FROM groups WHERE id = \$1`).
		WithArgs(groupID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "name", "description", "is_system", "created_at"}).
				AddRow(groupID, "devs", "Developers", false, now),
		)

	mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
		WithArgs(groupID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "username", "email"}).
				AddRow(userID, "alice", "alice@example.com"),
		)

	mock.ExpectQuery(`SELECT a\.id, a\.network_id, n\.name`).
		WithArgs(groupID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "network_id", "name", "allowed_ips"}),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+groupID.String(), nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	w := httptest.NewRecorder()

	h.get(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var g groupDetail
	require.NoError(t, json.NewDecoder(w.Body).Decode(&g))
	assert.Equal(t, "devs", g.Name)
	assert.Len(t, g.Members, 1)
	assert.Equal(t, "alice", g.Members[0].Username)
	assert.Empty(t, g.ACLs)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupGetNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()

	mock.ExpectQuery(`SELECT id, name, description, is_system, created_at\s+FROM groups WHERE id = \$1`).
		WithArgs(groupID).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/"+groupID.String(), nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	w := httptest.NewRecorder()

	h.get(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Delete ----

func TestGroupDeleteFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()

	mock.ExpectQuery(`SELECT is_system FROM groups WHERE id = \$1`).
		WithArgs(groupID).
		WillReturnRows(pgxmock.NewRows([]string{"is_system"}).AddRow(false))

	mock.ExpectExec(`DELETE FROM groups WHERE id = \$1`).
		WithArgs(groupID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/"+groupID.String(), nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupDeleteNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()

	mock.ExpectQuery(`SELECT is_system FROM groups WHERE id = \$1`).
		WithArgs(groupID).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodDelete, "/"+groupID.String(), nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- Members CRUD ----

func TestGroupAddMember(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	userID := uuid.New()

	mock.ExpectExec(`INSERT INTO user_groups`).
		WithArgs(userID, groupID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	body := fmt.Sprintf(`{"user_id":"%s"}`, userID)
	req := httptest.NewRequest(http.MethodPost, "/"+groupID.String()+"/members", bytes.NewBufferString(body))
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.addMember(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRemoveMember(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	userID := uuid.New()

	mock.ExpectExec(`DELETE FROM user_groups WHERE user_id = \$1 AND group_id = \$2`).
		WithArgs(userID, groupID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req = withChiParams(req, map[string]string{"id": groupID.String(), "userId": userID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.removeMember(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupListMembers(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	u1 := uuid.New()
	u2 := uuid.New()

	mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
		WithArgs(groupID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "username", "email"}).
				AddRow(u1, "alice", "alice@test.com").
				AddRow(u2, "bob", "bob@test.com"),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+groupID.String()+"/members", nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	w := httptest.NewRecorder()

	h.listMembers(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var members []groupMember
	require.NoError(t, json.NewDecoder(w.Body).Decode(&members))
	assert.Len(t, members, 2)
	assert.Equal(t, "alice", members[0].Username)

	require.NoError(t, mock.ExpectationsWereMet())
}

// ---- ACLs CRUD ----

func TestGroupAddACL(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	networkID := uuid.New()
	aclID := uuid.New()

	mock.ExpectQuery(`INSERT INTO network_acls`).
		WithArgs(networkID, groupID, []string{"10.0.0.0/8"}).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "network_id", "name", "allowed_ips"}).
				AddRow(aclID, networkID, "office-net", []string{"10.0.0.0/8"}),
		)

	body := fmt.Sprintf(`{"network_id":"%s","allowed_ips":["10.0.0.0/8"]}`, networkID)
	req := httptest.NewRequest(http.MethodPost, "/"+groupID.String()+"/acls", bytes.NewBufferString(body))
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.addACL(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var acl groupACL
	require.NoError(t, json.NewDecoder(w.Body).Decode(&acl))
	assert.Equal(t, networkID, acl.NetworkID)
	assert.Equal(t, "office-net", acl.NetworkName)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRemoveACL(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	aclID := uuid.New()

	mock.ExpectExec(`DELETE FROM network_acls WHERE id = \$1 AND group_id = \$2`).
		WithArgs(aclID, groupID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req = withChiParams(req, map[string]string{"id": groupID.String(), "aclId": aclID.String()})
	req = adminCtx(req)
	w := httptest.NewRecorder()

	h.removeACL(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupListACLs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	h := NewGroupHandler(mock)
	groupID := uuid.New()
	aclID := uuid.New()
	netID := uuid.New()

	mock.ExpectQuery(`SELECT a\.id, a\.network_id, n\.name`).
		WithArgs(groupID).
		WillReturnRows(
			pgxmock.NewRows([]string{"id", "network_id", "name", "allowed_ips"}).
				AddRow(aclID, netID, "prod-net", []string{"10.0.0.0/8"}),
		)

	req := httptest.NewRequest(http.MethodGet, "/"+groupID.String()+"/acls", nil)
	req = withChiParams(req, map[string]string{"id": groupID.String()})
	w := httptest.NewRecorder()

	h.listACLs(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var acls []groupACL
	require.NoError(t, json.NewDecoder(w.Body).Decode(&acls))
	assert.Len(t, acls, 1)
	assert.Equal(t, "prod-net", acls[0].NetworkName)

	require.NoError(t, mock.ExpectationsWereMet())
}
