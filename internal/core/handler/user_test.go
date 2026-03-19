package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/romashqua/outpost/internal/auth"
)

// --- test helpers (prefixed to avoid conflicts with other *_test.go) ---

func uReqWithClaims(r *http.Request, claims *auth.TokenClaims) *http.Request {
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}

func uWithChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func uAdminClaims() *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:  uuid.New().String(),
		IsAdmin: true,
	}
}

func uNonAdminClaims(userID string) *auth.TokenClaims {
	return &auth.TokenClaims{
		UserID:  userID,
		IsAdmin: false,
	}
}

var uTestLogger = slog.Default()

func uUserColumns() []string {
	return []string{"id", "username", "email", "first_name", "last_name", "phone",
		"is_active", "is_admin", "mfa_enabled", "last_login", "created_at", "updated_at"}
}

func uMakeUserRow(id uuid.UUID, username, email, firstName, lastName string, isActive, isAdmin bool) []any {
	now := time.Now()
	return []any{id, username, email, firstName, lastName, (*string)(nil),
		isActive, isAdmin, false, (*time.Time)(nil), now, now}
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func TestUserList(t *testing.T) {
	t.Run("returns paginated user list", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id1 := uuid.New()
		id2 := uuid.New()

		mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
			WithArgs(50, 0).
			WillReturnRows(
				pgxmock.NewRows(uUserColumns()).
					AddRow(uMakeUserRow(id1, "alice", "alice@test.com", "Alice", "A", true, true)...).
					AddRow(uMakeUserRow(id2, "bob", "bob@test.com", "Bob", "B", true, false)...),
			)
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users", nil)
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.list(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		users := body["users"].([]any)
		assert.Len(t, users, 2)
		assert.Equal(t, float64(2), body["total"])
		assert.Equal(t, float64(1), body["page"])
		assert.Equal(t, float64(50), body["per_page"])

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("returns empty list", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
			WithArgs(50, 0).
			WillReturnRows(pgxmock.NewRows(uUserColumns()))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users", nil)
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.list(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var body map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		users := body["users"].([]any)
		assert.Len(t, users, 0)
		assert.Equal(t, float64(0), body["total"])

		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func TestUserCreate(t *testing.T) {
	t.Run("successful creation by admin", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		newID := uuid.New()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO users`).
			WithArgs("newuser", "new@test.com", pgxmock.AnyArg(), "New", "User", false).
			WillReturnRows(pgxmock.NewRows(
				[]string{"id", "username", "email", "first_name", "last_name", "phone",
					"is_active", "is_admin", "mfa_enabled", "created_at", "updated_at"}).
				AddRow(newID, "newuser", "new@test.com", "New", "User", (*string)(nil),
					true, false, false, time.Now(), time.Now()))
		mock.ExpectExec(`INSERT INTO user_roles`).
			WithArgs(newID, "user").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectExec(`INSERT INTO user_groups`).
			WithArgs(newID).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"newuser","email":"new@test.com","password":"P@ssw0rd!","first_name":"New","last_name":"User"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp userResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "newuser", resp.Username)
		assert.Equal(t, "new@test.com", resp.Email)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("missing username", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"email":"a@b.com","password":"P@ssw0rd!"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "username is required")
	})

	t.Run("missing email", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"test","password":"P@ssw0rd!"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "email is required")
	})

	t.Run("missing password", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"test","email":"a@b.com"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "password is required")
	})

	t.Run("duplicate username returns 409", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO users`).
			WithArgs("dup", "dup@test.com", pgxmock.AnyArg(), "D", "U", false).
			WillReturnError(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: "users_username_key",
			})
		mock.ExpectRollback()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"dup","email":"dup@test.com","password":"P@ssw0rd!","first_name":"D","last_name":"U"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "username already exists")
	})

	t.Run("duplicate email returns 409", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		mock.ExpectBegin()
		mock.ExpectQuery(`INSERT INTO users`).
			WithArgs("unique", "dup@test.com", pgxmock.AnyArg(), "D", "U", false).
			WillReturnError(&pgconn.PgError{
				Code:           "23505",
				ConstraintName: "users_email_key",
			})
		mock.ExpectRollback()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"unique","email":"dup@test.com","password":"P@ssw0rd!","first_name":"D","last_name":"U"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.create(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "email already exists")
	})

	t.Run("non-admin rejected via RequireAdmin middleware", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		h := NewUserHandler(mock, uTestLogger)
		body := `{"username":"x","email":"x@test.com","password":"P@ssw0rd!","first_name":"X","last_name":"X"}`
		r := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uReqWithClaims(r, uNonAdminClaims(uuid.New().String()))
		w := httptest.NewRecorder()

		// Use the full router to exercise the RequireAdmin middleware.
		router := chi.NewRouter()
		router.Mount("/users", h.Routes())
		router.ServeHTTP(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "admin access required")
	})
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func TestUserGet(t *testing.T) {
	t.Run("successful get by admin", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows(uUserColumns()).
				AddRow(uMakeUserRow(id, "alice", "alice@test.com", "Alice", "A", true, false)...))

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.get(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp userResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id, resp.ID)
		assert.Equal(t, "alice", resp.Username)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user can view own profile", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows(uUserColumns()).
				AddRow(uMakeUserRow(id, "bob", "bob@test.com", "Bob", "B", true, false)...))

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uNonAdminClaims(id.String()))
		w := httptest.NewRecorder()

		h.get(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-admin cannot view other user", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		targetID := uuid.New()

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users/"+targetID.String(), nil)
		r = uWithChiParam(r, "id", targetID.String())
		r = uReqWithClaims(r, uNonAdminClaims(uuid.New().String())) // different user
		w := httptest.NewRecorder()

		h.get(w, r)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "access denied")
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectQuery(`SELECT u\.id, u\.username, u\.email`).
			WithArgs(id).
			WillReturnError(pgx.ErrNoRows)

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodGet, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.get(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "user not found")

		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func TestUserUpdate(t *testing.T) {
	t.Run("successful full update", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()
		newEmail := "updated@test.com"
		newFirst := "Updated"
		newLast := "Name"

		mock.ExpectQuery(`UPDATE users SET`).
			WithArgs(id, &newEmail, &newFirst, &newLast, (*string)(nil), (*bool)(nil), (*bool)(nil)).
			WillReturnRows(pgxmock.NewRows(uUserColumns()).
				AddRow(uMakeUserRow(id, "alice", newEmail, newFirst, newLast, true, false)...))

		h := NewUserHandler(mock, uTestLogger)
		body := `{"email":"updated@test.com","first_name":"Updated","last_name":"Name"}`
		r := httptest.NewRequest(http.MethodPut, "/users/"+id.String(), bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.update(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp userResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "updated@test.com", resp.Email)
		assert.Equal(t, "Updated", resp.FirstName)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("partial update with COALESCE", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()
		newEmail := "partial@test.com"

		mock.ExpectQuery(`UPDATE users SET`).
			WithArgs(id, &newEmail, (*string)(nil), (*string)(nil), (*string)(nil), (*bool)(nil), (*bool)(nil)).
			WillReturnRows(pgxmock.NewRows(uUserColumns()).
				AddRow(uMakeUserRow(id, "alice", newEmail, "Alice", "A", true, false)...))

		h := NewUserHandler(mock, uTestLogger)
		body := `{"email":"partial@test.com"}`
		r := httptest.NewRequest(http.MethodPut, "/users/"+id.String(), bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.update(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()
		newEmail := "x@test.com"

		mock.ExpectQuery(`UPDATE users SET`).
			WithArgs(id, &newEmail, (*string)(nil), (*string)(nil), (*string)(nil), (*bool)(nil), (*bool)(nil)).
			WillReturnError(pgx.ErrNoRows)

		h := NewUserHandler(mock, uTestLogger)
		body := `{"email":"x@test.com"}`
		r := httptest.NewRequest(http.MethodPut, "/users/"+id.String(), bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.update(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "user not found")

		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func TestUserDelete(t *testing.T) {
	t.Run("successful delete of non-admin", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.Serializable})
		mock.ExpectQuery(`SELECT is_admin, is_active FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows([]string{"is_admin", "is_active"}).AddRow(false, true))
		mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodDelete, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.delete(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("successful delete of admin when not last admin", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.Serializable})
		mock.ExpectQuery(`SELECT is_admin, is_active FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows([]string{"is_admin", "is_active"}).AddRow(true, true))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE is_admin = true AND is_active = true`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))
		mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))
		mock.ExpectCommit()

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodDelete, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.delete(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("cannot delete last admin", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.Serializable})
		mock.ExpectQuery(`SELECT is_admin, is_active FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows([]string{"is_admin", "is_active"}).AddRow(true, true))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users WHERE is_admin = true AND is_active = true`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodDelete, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.delete(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "cannot delete the last admin")

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.Serializable})
		mock.ExpectQuery(`SELECT is_admin, is_active FROM users WHERE id = \$1`).
			WithArgs(id).
			WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodDelete, "/users/"+id.String(), nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.delete(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "user not found")

		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---------------------------------------------------------------------------
// activate
// ---------------------------------------------------------------------------

func TestUserActivate(t *testing.T) {
	t.Run("successful activation", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectQuery(`UPDATE users SET is_active = true`).
			WithArgs(id).
			WillReturnRows(pgxmock.NewRows(uUserColumns()).
				AddRow(uMakeUserRow(id, "bob", "bob@test.com", "Bob", "B", true, false)...))

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodPatch, "/users/"+id.String()+"/activate", nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.activate(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp userResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.True(t, resp.IsActive)
		assert.Equal(t, id, resp.ID)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mock, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer mock.Close()

		id := uuid.New()

		mock.ExpectQuery(`UPDATE users SET is_active = true`).
			WithArgs(id).
			WillReturnError(pgx.ErrNoRows)

		h := NewUserHandler(mock, uTestLogger)
		r := httptest.NewRequest(http.MethodPatch, "/users/"+id.String()+"/activate", nil)
		r = uWithChiParam(r, "id", id.String())
		r = uReqWithClaims(r, uAdminClaims())
		w := httptest.NewRecorder()

		h.activate(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "user not found")

		require.NoError(t, mock.ExpectationsWereMet())
	})
}
