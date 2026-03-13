package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// UserHandler provides CRUD endpoints for user management.
type UserHandler struct {
	pool *pgxpool.Pool
}

// NewUserHandler creates a UserHandler backed by the given connection pool.
func NewUserHandler(pool *pgxpool.Pool) *UserHandler {
	return &UserHandler{pool: pool}
}

// Routes returns a chi.Router with user CRUD endpoints mounted.
func (h *UserHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.Put("/", h.update)
		r.Delete("/", h.delete)
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
		"data":     users,
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
		respondError(w, http.StatusConflict, "user already exists or invalid data")
		return
	}

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
		respondError(w, http.StatusNotFound, "user not found")
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
		respondError(w, http.StatusNotFound, "user not found")
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

	tag, err := h.pool.Exec(r.Context(),
		`UPDATE users SET is_active = false, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to deactivate user")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
