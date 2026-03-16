package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// Mailer is the interface for sending emails (satisfied by mail.Mailer).
type Mailer interface {
	SendWelcome(ctx context.Context, to, username, instanceName string) error
	SendEnrollmentInvite(ctx context.Context, to, enrollURL, instanceName string) error
}

// UserHandler provides CRUD endpoints for user management.
type UserHandler struct {
	pool   *pgxpool.Pool
	log    *slog.Logger
	mailer Mailer
}

// NewUserHandler creates a UserHandler backed by the given connection pool.
// An optional mailer can be provided to send welcome emails on user creation.
func NewUserHandler(pool *pgxpool.Pool, logger *slog.Logger, mailer ...Mailer) *UserHandler {
	l := slog.Default()
	if logger != nil {
		l = logger
	}
	h := &UserHandler{pool: pool, log: l.With("handler", "user")}
	if len(mailer) > 0 {
		h.mailer = mailer[0]
	}
	return h
}

// Routes returns a chi.Router with user CRUD endpoints mounted.
// Write operations (create, update, delete, activate) require admin privileges.
func (h *UserHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.With(auth.RequireAdmin).Get("/", h.list)
	r.With(auth.RequireAdmin).Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.With(auth.RequireAdmin).Put("/", h.update)
		r.With(auth.RequireAdmin).Delete("/", h.delete)
		r.With(auth.RequireAdmin).Patch("/activate", h.activate)
	})
	return r
}

type userResponse struct {
	ID         uuid.UUID  `json:"id"`
	Username   string     `json:"username"`
	Email      string     `json:"email"`
	FirstName  string     `json:"first_name"`
	LastName   string     `json:"last_name"`
	Phone      *string    `json:"phone,omitempty"`
	IsActive   bool       `json:"is_active"`
	IsAdmin    bool       `json:"is_admin"`
	MFAEnabled bool       `json:"mfa_enabled"`
	LastLogin  *time.Time `json:"last_login"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type createUserRequest struct {
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	Password  string  `json:"password"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	IsAdmin   bool    `json:"is_admin"`
}

type updateUserRequest struct {
	Email     *string `json:"email,omitempty"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Phone     *string `json:"phone,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
	IsAdmin   *bool   `json:"is_admin,omitempty"`
}

// @Summary List users
// @Description Returns a paginated list of all users.
// @Tags Users
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param per_page query int false "Items per page" default(50)
// @Success 200 {object} map[string]any
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users [get]
func (h *UserHandler) list(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.username, u.email, u.first_name, u.last_name, u.phone, u.is_active, u.is_admin, u.mfa_enabled,
		        (SELECT MAX(s.created_at) FROM sessions s WHERE s.user_id = u.id),
		        u.created_at, u.updated_at
		 FROM users u
		 ORDER BY u.created_at DESC
		 LIMIT $1 OFFSET $2`, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query users")
		return
	}
	defer rows.Close()

	users := make([]userResponse, 0)
	for rows.Next() {
		var u userResponse
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
			&u.Phone, &u.IsActive, &u.IsAdmin, &u.MFAEnabled, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan user")
			return
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate users")
		return
	}

	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to count users")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"users":    users,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// @Summary Create user
// @Description Create a new user account. Requires admin privileges.
// @Tags Users
// @Accept json
// @Produce json
// @Param body body createUserRequest true "User data"
// @Success 201 {object} userResponse
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users [post]
func (h *UserHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Username == "" {
		respondError(w, http.StatusBadRequest, "username is required")
		return
	}
	if req.Email == "" {
		respondError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Password == "" {
		respondError(w, http.StatusBadRequest, "password is required")
		return
	}
	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO users (username, email, password_hash, first_name, last_name, is_admin)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, mfa_enabled, created_at, updated_at`,
		req.Username, req.Email, hash, req.FirstName, req.LastName, req.IsAdmin,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.MFAEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			msg := "user already exists"
			if strings.Contains(pgErr.ConstraintName, "email") {
				msg = "user with this email already exists"
			} else if strings.Contains(pgErr.ConstraintName, "username") {
				msg = "user with this username already exists"
			}
			respondError(w, http.StatusConflict, msg)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Assign role based on is_admin flag.
	roleName := "user"
	if req.IsAdmin {
		roleName = "admin"
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO user_roles (user_id, role_id)
		 SELECT $1, id FROM roles WHERE name = $2
		 ON CONFLICT DO NOTHING`, u.ID, roleName); err != nil {
		h.log.Error("failed to assign role to user", "user_id", u.ID, "role", roleName, "error", err)
	}

	// Send welcome email asynchronously (best-effort, 30s timeout).
	if h.mailer != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.mailer.SendWelcome(ctx, u.Email, u.Username, "Outpost VPN"); err != nil {
				h.log.Error("failed to send welcome email", "user", u.Username, "error", err)
			}
		}()
	}

	h.log.Info("user created", "id", u.ID, "username", u.Username, "email", u.Email, "is_admin", u.IsAdmin)
	respondJSON(w, http.StatusCreated, u)
}

// @Summary Get user
// @Description Retrieve a user by ID.
// @Tags Users
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Success 200 {object} userResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id} [get]
func (h *UserHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// IDOR check: non-admins can only view their own profile.
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if !claims.IsAdmin && claims.UserID != id.String() {
		respondError(w, http.StatusForbidden, "access denied")
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT u.id, u.username, u.email, u.first_name, u.last_name, u.phone, u.is_active, u.is_admin, u.mfa_enabled,
		        (SELECT MAX(s.created_at) FROM sessions s WHERE s.user_id = u.id),
		        u.created_at, u.updated_at
		 FROM users u WHERE u.id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.MFAEnabled, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "user not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch user")
		}
		return
	}

	respondJSON(w, http.StatusOK, u)
}

// @Summary Update user
// @Description Update an existing user. Requires admin privileges.
// @Tags Users
// @Accept json
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Param body body updateUserRequest true "Fields to update"
// @Success 200 {object} userResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id} [put]
func (h *UserHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateUserRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`UPDATE users SET
			email      = COALESCE($2, email),
			first_name = COALESCE($3, first_name),
			last_name  = COALESCE($4, last_name),
			phone      = COALESCE($5, phone),
			is_active  = COALESCE($6, is_active),
			is_admin   = COALESCE($7, is_admin),
			updated_at = now()
		 WHERE id = $1
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, mfa_enabled,
		           (SELECT MAX(s.created_at) FROM sessions s WHERE s.user_id = id),
		           created_at, updated_at`,
		id, req.Email, req.FirstName, req.LastName, req.Phone, req.IsActive, req.IsAdmin,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.MFAEnabled, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "user not found")
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				msg := "user already exists"
				if strings.Contains(pgErr.ConstraintName, "email") {
					msg = "user with this email already exists"
				} else if strings.Contains(pgErr.ConstraintName, "username") {
					msg = "user with this username already exists"
				}
				respondError(w, http.StatusConflict, msg)
			} else {
				respondError(w, http.StatusInternalServerError, "failed to update user")
			}
		}
		return
	}

	respondJSON(w, http.StatusOK, u)
}

// @Summary Delete user
// @Description Delete a user by ID. Cannot delete the last active admin. Requires admin privileges.
// @Tags Users
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id} [delete]
func (h *UserHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()

	// Use a serializable transaction to atomically check the admin count and
	// delete the user. This prevents a race condition where two concurrent
	// admin deletions could both pass the "last admin" check and leave the
	// system with no admins.
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	defer tx.Rollback(ctx)

	// Check whether the target user is an active admin. If so, ensure they
	// are not the last one — deleting the last admin would lock everyone out.
	var isAdmin, isActive bool
	err = tx.QueryRow(ctx,
		`SELECT is_admin, is_active FROM users WHERE id = $1`, id,
	).Scan(&isAdmin, &isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "user not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch user")
		}
		return
	}

	if isAdmin && isActive {
		var activeAdminCount int
		err = tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM users WHERE is_admin = true AND is_active = true`,
		).Scan(&activeAdminCount)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to count admins")
			return
		}
		if activeAdminCount <= 1 {
			respondError(w, http.StatusConflict, "cannot delete the last admin user")
			return
		}
	}

	tag, err := tx.Exec(ctx,
		`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	if err = tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	h.log.Info("user deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// @Summary Activate user
// @Description Activate a deactivated user account. Requires admin privileges.
// @Tags Users
// @Produce json
// @Param id path string true "User ID (UUID)"
// @Success 200 {object} userResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /users/{id}/activate [patch]
func (h *UserHandler) activate(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`UPDATE users SET is_active = true, updated_at = now()
		 WHERE id = $1
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, mfa_enabled,
		           (SELECT MAX(s.created_at) FROM sessions s WHERE s.user_id = id),
		           created_at, updated_at`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.MFAEnabled, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "user not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to activate user")
		}
		return
	}

	respondJSON(w, http.StatusOK, u)
}
