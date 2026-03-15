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
	r.Get("/", h.list)
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
	ID        uuid.UUID  `json:"id"`
	Username  string     `json:"username"`
	Email     string     `json:"email"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Phone     *string    `json:"phone,omitempty"`
	IsActive  bool       `json:"is_active"`
	IsAdmin   bool       `json:"is_admin"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
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

func (h *UserHandler) list(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePagination(r)
	offset := (page - 1) * perPage

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, username, email, first_name, last_name, phone, is_active, is_admin, created_at, updated_at
		 FROM users
		 ORDER BY created_at DESC
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
			&u.Phone, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan user")
			return
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate users")
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"users":    users,
		"page":     page,
		"per_page": perPage,
	})
}

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

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO users (username, email, password_hash, first_name, last_name, is_admin)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, created_at, updated_at`,
		req.Username, req.Email, hash, req.FirstName, req.LastName, req.IsAdmin,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
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

	// Send welcome email asynchronously (best-effort).
	if h.mailer != nil {
		go func() {
			if err := h.mailer.SendWelcome(context.Background(), u.Email, u.Username, "Outpost VPN"); err != nil {
				h.log.Error("failed to send welcome email", "user", u.Username, "error", err)
			}
		}()
	}

	h.log.Info("user created", "id", u.ID, "username", u.Username, "email", u.Email, "is_admin", u.IsAdmin)
	respondJSON(w, http.StatusCreated, u)
}

func (h *UserHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var u userResponse
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, username, email, first_name, last_name, phone, is_active, is_admin, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
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
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, created_at, updated_at`,
		id, req.Email, req.FirstName, req.LastName, req.Phone, req.IsActive, req.IsAdmin,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
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

func (h *UserHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()

	// Check whether the target user is an active admin. If so, ensure they
	// are not the last one — deleting the last admin would lock everyone out.
	var isAdmin, isActive bool
	err = h.pool.QueryRow(ctx,
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
		err = h.pool.QueryRow(ctx,
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

	tag, err := h.pool.Exec(ctx,
		`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	h.log.Info("user deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

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
		 RETURNING id, username, email, first_name, last_name, phone, is_active, is_admin, created_at, updated_at`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsActive, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
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
