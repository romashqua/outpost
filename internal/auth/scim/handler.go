// Package scim implements SCIM 2.0 provisioning endpoints (RFC 7643/7644)
// for Outpost VPN. These endpoints allow identity providers like Okta and
// Azure AD to automatically provision and de-provision users and groups.
package scim

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

const (
	// SCIM media type per RFC 7644 section 3.1.
	scimMediaType = "application/scim+json"

	// SCIM schema URIs.
	schemaUser              = "urn:ietf:params:scim:schemas:core:2.0:User"
	schemaGroup             = "urn:ietf:params:scim:schemas:core:2.0:Group"
	schemaListResponse      = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	schemaError             = "urn:ietf:params:scim:api:messages:2.0:Error"
	schemaPatchOp           = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	schemaServiceProviderCfg = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	schemaSchema            = "urn:ietf:params:scim:schemas:core:2.0:Schema"

	defaultCount = 100
)

// Handler provides SCIM 2.0 provisioning endpoints.
type Handler struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewHandler creates a new SCIM handler.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, logger: logger}
}

// bearerTokenAuth is a middleware that validates SCIM bearer tokens.
// The token is stored in the settings table under key "scim_token".
func (h *Handler) bearerTokenAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			respondSCIMError(w, http.StatusUnauthorized, "Bearer token required")
			return
		}
		token := authHeader[7:]

		var storedToken string
		err := h.pool.QueryRow(r.Context(),
			`SELECT value::text FROM settings WHERE key = 'scim_token'`,
		).Scan(&storedToken)
		if err != nil {
			respondSCIMError(w, http.StatusForbidden, "SCIM provisioning is not configured")
			return
		}
		// Remove surrounding quotes from JSONB text cast.
		storedToken = trimJSONString(storedToken)

		if subtle.ConstantTimeCompare([]byte(token), []byte(storedToken)) != 1 {
			respondSCIMError(w, http.StatusUnauthorized, "Invalid bearer token")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// trimJSONString removes surrounding double-quotes from a JSONB string value.
func trimJSONString(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// Routes returns a chi.Router with all SCIM 2.0 endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Bearer token authentication for all SCIM endpoints.
	r.Use(h.bearerTokenAuth)

	// User endpoints.
	r.Get("/Users", h.listUsers)
	r.Post("/Users", h.createUser)
	r.Route("/Users/{id}", func(r chi.Router) {
		r.Get("/", h.getUser)
		r.Put("/", h.replaceUser)
		r.Patch("/", h.patchUser)
		r.Delete("/", h.deleteUser)
	})

	// Group endpoints.
	r.Get("/Groups", h.listGroups)
	r.Post("/Groups", h.createGroup)
	r.Route("/Groups/{id}", func(r chi.Router) {
		r.Get("/", h.getGroup)
		r.Put("/", h.replaceGroup)
		r.Patch("/", h.patchGroup)
		r.Delete("/", h.deleteGroup)
	})

	// Discovery endpoints.
	r.Get("/ServiceProviderConfig", h.serviceProviderConfig)
	r.Get("/Schemas", h.schemas)

	return r
}

// --- SCIM Resource Types ---

// SCIMUser represents a SCIM 2.0 User resource (RFC 7643 section 4.1).
type SCIMUser struct {
	Schemas    []string    `json:"schemas"`
	ID         string      `json:"id"`
	ExternalID string      `json:"externalId,omitempty"`
	UserName   string      `json:"userName"`
	Name       *SCIMName   `json:"name,omitempty"`
	Emails     []SCIMEmail `json:"emails,omitempty"`
	Active     bool        `json:"active"`
	Meta       SCIMMeta    `json:"meta"`
}

// SCIMName represents a user's name components.
type SCIMName struct {
	GivenName  string `json:"givenName,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
	Formatted  string `json:"formatted,omitempty"`
}

// SCIMEmail represents an email address entry.
type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// SCIMMeta holds resource metadata per RFC 7643 section 3.1.
type SCIMMeta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
}

// SCIMGroup represents a SCIM 2.0 Group resource.
type SCIMGroup struct {
	Schemas []string     `json:"schemas"`
	ID      string       `json:"id"`
	Name    string       `json:"displayName"`
	Members []SCIMMember `json:"members,omitempty"`
	Meta    SCIMMeta     `json:"meta"`
}

// SCIMMember represents a group member reference.
type SCIMMember struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

// SCIMListResponse wraps a list of SCIM resources (RFC 7644 section 3.4.2).
type SCIMListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    any      `json:"Resources"`
}

// SCIMError represents a SCIM error response (RFC 7644 section 3.12).
type SCIMError struct {
	Schemas []string `json:"schemas"`
	Detail  string   `json:"detail"`
	Status  string   `json:"status"`
}

// SCIMPatchOp represents a SCIM PATCH request (RFC 7644 section 3.5.2).
type SCIMPatchOp struct {
	Schemas    []string        `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}

// PatchOperation represents a single SCIM PATCH operation.
type PatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}

// CreateUserRequest is the incoming SCIM user creation payload.
type CreateUserRequest struct {
	Schemas    []string    `json:"schemas"`
	ExternalID string      `json:"externalId,omitempty"`
	UserName   string      `json:"userName"`
	Name       *SCIMName   `json:"name,omitempty"`
	Emails     []SCIMEmail `json:"emails,omitempty"`
	Active     *bool       `json:"active,omitempty"`
	Password   string      `json:"password,omitempty"`
}

// --- Response Helpers ---

func respondSCIM(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", scimMediaType)
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondSCIMError(w http.ResponseWriter, status int, detail string) {
	respondSCIM(w, status, SCIMError{
		Schemas: []string{schemaError},
		Detail:  detail,
		Status:  strconv.Itoa(status),
	})
}

func parseSCIMBody(r *http.Request, dst any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}
	limited := io.LimitReader(r.Body, 1<<20) // 1MB limit
	if err := json.NewDecoder(limited).Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// parseSCIMPagination extracts startIndex and count from SCIM query params.
func parseSCIMPagination(r *http.Request) (startIndex, count int) {
	startIndex = queryIntDefault(r, "startIndex", 1)
	count = queryIntDefault(r, "count", defaultCount)
	if startIndex < 1 {
		startIndex = 1
	}
	if count < 1 {
		count = defaultCount
	}
	if count > 1000 {
		count = 1000
	}
	return startIndex, count
}

func queryIntDefault(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// dbUserToSCIM converts database user fields into a SCIM User resource.
func dbUserToSCIM(id uuid.UUID, username, email, firstName, lastName string, isActive bool,
	externalID *string, createdAt, updatedAt time.Time) SCIMUser {
	u := SCIMUser{
		Schemas:  []string{schemaUser},
		ID:       id.String(),
		UserName: username,
		Active:   isActive,
		Name: &SCIMName{
			GivenName:  firstName,
			FamilyName: lastName,
			Formatted:  firstName + " " + lastName,
		},
		Meta: SCIMMeta{
			ResourceType: "User",
			Created:      createdAt.UTC().Format(time.RFC3339),
			LastModified: updatedAt.UTC().Format(time.RFC3339),
		},
	}
	if email != "" {
		u.Emails = []SCIMEmail{{Value: email, Type: "work", Primary: true}}
	}
	if externalID != nil {
		u.ExternalID = *externalID
	}
	return u
}

// --- User Endpoints ---

// @Summary List SCIM users
// @Description Return a paginated list of SCIM 2.0 User resources (RFC 7644 section 3.4.2).
// @Tags SCIM
// @Produce json
// @Param startIndex query int false "1-based start index" default(1)
// @Param count query int false "Maximum number of results" default(100)
// @Success 200 {object} SCIMListResponse
// @Failure 401 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users [get]
func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	startIndex, count := parseSCIMPagination(r)
	offset := startIndex - 1 // SCIM is 1-indexed

	// Retrieve total count.
	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&total); err != nil {
		h.logger.Error("scim: counting users", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to count users")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, username, email, first_name, last_name, is_active, scim_external_id, created_at, updated_at
		 FROM users
		 ORDER BY created_at
		 LIMIT $1 OFFSET $2`, count, offset)
	if err != nil {
		h.logger.Error("scim: listing users", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	defer rows.Close()

	users := make([]SCIMUser, 0)
	for rows.Next() {
		var (
			id         uuid.UUID
			username   string
			email      string
			firstName  string
			lastName   string
			isActive   bool
			externalID *string
			createdAt  time.Time
			updatedAt  time.Time
		)
		if err := rows.Scan(&id, &username, &email, &firstName, &lastName,
			&isActive, &externalID, &createdAt, &updatedAt); err != nil {
			h.logger.Error("scim: scanning user", "error", err)
			respondSCIMError(w, http.StatusInternalServerError, "failed to read user")
			return
		}
		users = append(users, dbUserToSCIM(id, username, email, firstName, lastName, isActive, externalID, createdAt, updatedAt))
	}
	if err := rows.Err(); err != nil {
		h.logger.Error("scim: iterating users", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to iterate users")
		return
	}

	respondSCIM(w, http.StatusOK, SCIMListResponse{
		Schemas:      []string{schemaListResponse},
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: len(users),
		Resources:    users,
	})
}

// @Summary Create SCIM user
// @Description Provision a new user via SCIM 2.0 (RFC 7644 section 3.3).
// @Tags SCIM
// @Accept json
// @Produce json
// @Param body body CreateUserRequest true "SCIM User resource"
// @Success 201 {object} SCIMUser
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 409 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users [post]
func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := parseSCIMBody(r, &req); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.UserName == "" {
		respondSCIMError(w, http.StatusBadRequest, "userName is required")
		return
	}

	// Extract primary email.
	var email string
	for _, e := range req.Emails {
		if e.Primary || email == "" {
			email = e.Value
		}
	}

	var firstName, lastName string
	if req.Name != nil {
		firstName = req.Name.GivenName
		lastName = req.Name.FamilyName
	}

	isActive := true
	if req.Active != nil {
		isActive = *req.Active
	}

	// Generate a random password for SCIM-provisioned users.
	password := req.Password
	if password == "" {
		var rb [16]byte
		_, _ = rand.Read(rb[:])
		password = "scim-" + hex.EncodeToString(rb[:])
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		h.logger.Error("scim: hashing password", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	var (
		id        uuid.UUID
		createdAt time.Time
		updatedAt time.Time
	)
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO users (username, email, password_hash, first_name, last_name, is_active, scim_external_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, created_at, updated_at`,
		req.UserName, email, hash, firstName, lastName, isActive, strPtrOrNil(req.ExternalID),
	).Scan(&id, &createdAt, &updatedAt)
	if err != nil {
		h.logger.Error("scim: creating user", "error", err, "username", req.UserName)
		respondSCIMError(w, http.StatusConflict, "user already exists or invalid data")
		return
	}

	h.logger.Info("scim: user created", "id", id, "username", req.UserName)
	user := dbUserToSCIM(id, req.UserName, email, firstName, lastName, isActive, strPtrOrNil(req.ExternalID), createdAt, updatedAt)
	respondSCIM(w, http.StatusCreated, user)
}

// @Summary Get SCIM user
// @Description Retrieve a single SCIM 2.0 User resource by ID.
// @Tags SCIM
// @Produce json
// @Param id path string true "User UUID"
// @Success 200 {object} SCIMUser
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users/{id} [get]
func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var (
		username   string
		email      string
		firstName  string
		lastName   string
		isActive   bool
		externalID *string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = h.pool.QueryRow(r.Context(),
		`SELECT username, email, first_name, last_name, is_active, scim_external_id, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&username, &email, &firstName, &lastName, &isActive, &externalID, &createdAt, &updatedAt)
	if err != nil {
		respondSCIMError(w, http.StatusNotFound, "user not found")
		return
	}

	respondSCIM(w, http.StatusOK, dbUserToSCIM(id, username, email, firstName, lastName, isActive, externalID, createdAt, updatedAt))
}

// @Summary Replace SCIM user
// @Description Fully replace a SCIM 2.0 User resource (RFC 7644 section 3.5.1).
// @Tags SCIM
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body CreateUserRequest true "SCIM User resource"
// @Success 200 {object} SCIMUser
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users/{id} [put]
func (h *Handler) replaceUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var req CreateUserRequest
	if err := parseSCIMBody(r, &req); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}

	var email string
	for _, e := range req.Emails {
		if e.Primary || email == "" {
			email = e.Value
		}
	}

	var firstName, lastName string
	if req.Name != nil {
		firstName = req.Name.GivenName
		lastName = req.Name.FamilyName
	}

	isActive := true
	if req.Active != nil {
		isActive = *req.Active
	}

	var (
		createdAt  time.Time
		updatedAt  time.Time
		externalID *string
	)
	err = h.pool.QueryRow(r.Context(),
		`UPDATE users SET
			username          = $2,
			email             = $3,
			first_name        = $4,
			last_name         = $5,
			is_active         = $6,
			scim_external_id  = $7,
			updated_at        = now()
		 WHERE id = $1
		 RETURNING created_at, updated_at, scim_external_id`,
		id, req.UserName, email, firstName, lastName, isActive, strPtrOrNil(req.ExternalID),
	).Scan(&createdAt, &updatedAt, &externalID)
	if err != nil {
		respondSCIMError(w, http.StatusNotFound, "user not found")
		return
	}

	h.logger.Info("scim: user replaced", "id", id, "username", req.UserName)
	respondSCIM(w, http.StatusOK, dbUserToSCIM(id, req.UserName, email, firstName, lastName, isActive, externalID, createdAt, updatedAt))
}

// @Summary Patch SCIM user
// @Description Apply partial updates to a SCIM 2.0 User resource (RFC 7644 section 3.5.2).
// @Tags SCIM
// @Accept json
// @Produce json
// @Param id path string true "User UUID"
// @Param body body SCIMPatchOp true "SCIM PatchOp request"
// @Success 200 {object} SCIMUser
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users/{id} [patch]
func (h *Handler) patchUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	var patch SCIMPatchOp
	if err := parseSCIMBody(r, &patch); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Process each patch operation.
	for _, op := range patch.Operations {
		switch op.Op {
		case "replace", "Replace":
			if err := h.applyUserReplace(r.Context(), id, op); err != nil {
				h.logger.Error("scim: patch replace failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "add", "Add":
			if err := h.applyUserReplace(r.Context(), id, op); err != nil {
				h.logger.Error("scim: patch add failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "remove", "Remove":
			// Handle remove by setting the field to empty/null.
			op.Value = nil
			if err := h.applyUserReplace(r.Context(), id, op); err != nil {
				h.logger.Error("scim: patch remove failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		default:
			respondSCIMError(w, http.StatusBadRequest, fmt.Sprintf("unsupported operation: %s", op.Op))
			return
		}
	}

	h.logger.Info("scim: user patched", "id", id)

	// Return the updated user.
	h.getUser(w, r)
}

// applyUserReplace applies a replace/add patch operation to a user.
func (h *Handler) applyUserReplace(ctx context.Context, id uuid.UUID, op PatchOperation) error {
	switch op.Path {
	case "active", "Active":
		active, ok := op.Value.(bool)
		if !ok {
			return fmt.Errorf("invalid value for active: expected boolean")
		}
		_, err := h.pool.Exec(ctx,
			`UPDATE users SET is_active = $2, updated_at = now() WHERE id = $1`, id, active)
		return err

	case "userName":
		val, ok := op.Value.(string)
		if !ok {
			return fmt.Errorf("invalid value for userName: expected string")
		}
		_, err := h.pool.Exec(ctx,
			`UPDATE users SET username = $2, updated_at = now() WHERE id = $1`, id, val)
		return err

	case "name.givenName":
		val, ok := op.Value.(string)
		if !ok {
			return fmt.Errorf("invalid value for name.givenName: expected string")
		}
		_, err := h.pool.Exec(ctx,
			`UPDATE users SET first_name = $2, updated_at = now() WHERE id = $1`, id, val)
		return err

	case "name.familyName":
		val, ok := op.Value.(string)
		if !ok {
			return fmt.Errorf("invalid value for name.familyName: expected string")
		}
		_, err := h.pool.Exec(ctx,
			`UPDATE users SET last_name = $2, updated_at = now() WHERE id = $1`, id, val)
		return err

	case "emails":
		// Handle emails as an array — extract primary email.
		emails, err := patchValueToEmails(op.Value)
		if err != nil {
			return err
		}
		var primary string
		for _, e := range emails {
			if e.Primary || primary == "" {
				primary = e.Value
			}
		}
		_, err = h.pool.Exec(ctx,
			`UPDATE users SET email = $2, updated_at = now() WHERE id = $1`, id, primary)
		return err

	case "externalId":
		val, _ := op.Value.(string)
		_, err := h.pool.Exec(ctx,
			`UPDATE users SET scim_external_id = $2, updated_at = now() WHERE id = $1`, id, nilIfEmpty(&val))
		return err

	case "":
		// No path — the value is a map of attributes to set.
		valueMap, ok := op.Value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object value when path is empty")
		}
		for key, val := range valueMap {
			subOp := PatchOperation{Op: op.Op, Path: key, Value: val}
			if err := h.applyUserReplace(ctx, id, subOp); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported patch path: %s", op.Path)
	}
}

// patchValueToEmails converts a PATCH value to a slice of SCIMEmail.
func patchValueToEmails(value any) ([]SCIMEmail, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid emails value: %w", err)
	}
	var emails []SCIMEmail
	if err := json.Unmarshal(raw, &emails); err != nil {
		return nil, fmt.Errorf("invalid emails format: %w", err)
	}
	return emails, nil
}

// @Summary Delete SCIM user
// @Description Deactivate a user via SCIM 2.0 DELETE (sets is_active=false).
// @Tags SCIM
// @Param id path string true "User UUID"
// @Success 204 "No Content"
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Users/{id} [delete]
func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid user ID")
		return
	}

	// SCIM DELETE deactivates, not hard-deletes.
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE users SET is_active = false, updated_at = now() WHERE id = $1`, id)
	if err != nil {
		h.logger.Error("scim: deleting user", "error", err, "id", id)
		respondSCIMError(w, http.StatusInternalServerError, "failed to deactivate user")
		return
	}
	if tag.RowsAffected() == 0 {
		respondSCIMError(w, http.StatusNotFound, "user not found")
		return
	}

	h.logger.Info("scim: user deactivated", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Group Endpoints ---

// dbGroupToSCIM converts database group fields into a SCIM Group resource.
func dbGroupToSCIM(id uuid.UUID, name string, createdAt time.Time, members []SCIMMember) SCIMGroup {
	return SCIMGroup{
		Schemas: []string{schemaGroup},
		ID:      id.String(),
		Name:    name,
		Members: members,
		Meta: SCIMMeta{
			ResourceType: "Group",
			Created:      createdAt.UTC().Format(time.RFC3339),
			LastModified: createdAt.UTC().Format(time.RFC3339),
		},
	}
}

// loadGroupMembers fetches all members of a group from the user_groups junction table.
func (h *Handler) loadGroupMembers(ctx context.Context, groupID uuid.UUID) ([]SCIMMember, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT u.id, u.username
		 FROM user_groups ug
		 JOIN users u ON u.id = ug.user_id
		 WHERE ug.group_id = $1
		 ORDER BY u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]SCIMMember, 0)
	for rows.Next() {
		var (
			uid      uuid.UUID
			username string
		)
		if err := rows.Scan(&uid, &username); err != nil {
			return nil, err
		}
		members = append(members, SCIMMember{
			Value:   uid.String(),
			Display: username,
		})
	}
	return members, rows.Err()
}

// @Summary List SCIM groups
// @Description Return a paginated list of SCIM 2.0 Group resources with members.
// @Tags SCIM
// @Produce json
// @Param startIndex query int false "1-based start index" default(1)
// @Param count query int false "Maximum number of results" default(100)
// @Success 200 {object} SCIMListResponse
// @Failure 401 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups [get]
func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	startIndex, count := parseSCIMPagination(r)
	offset := startIndex - 1

	var total int
	if err := h.pool.QueryRow(r.Context(), `SELECT count(*) FROM groups`).Scan(&total); err != nil {
		h.logger.Error("scim: counting groups", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to count groups")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, created_at
		 FROM groups
		 ORDER BY created_at
		 LIMIT $1 OFFSET $2`, count, offset)
	if err != nil {
		h.logger.Error("scim: listing groups", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to list groups")
		return
	}
	defer rows.Close()

	groups := make([]SCIMGroup, 0)
	for rows.Next() {
		var (
			id        uuid.UUID
			name      string
			createdAt time.Time
		)
		if err := rows.Scan(&id, &name, &createdAt); err != nil {
			h.logger.Error("scim: scanning group", "error", err)
			respondSCIMError(w, http.StatusInternalServerError, "failed to read group")
			return
		}
		members, err := h.loadGroupMembers(r.Context(), id)
		if err != nil {
			h.logger.Error("scim: loading group members", "error", err, "group_id", id)
			respondSCIMError(w, http.StatusInternalServerError, "failed to load group members")
			return
		}
		groups = append(groups, dbGroupToSCIM(id, name, createdAt, members))
	}
	if err := rows.Err(); err != nil {
		h.logger.Error("scim: iterating groups", "error", err)
		respondSCIMError(w, http.StatusInternalServerError, "failed to iterate groups")
		return
	}

	respondSCIM(w, http.StatusOK, SCIMListResponse{
		Schemas:      []string{schemaListResponse},
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: len(groups),
		Resources:    groups,
	})
}

// @Summary Create SCIM group
// @Description Provision a new group via SCIM 2.0 with optional initial members.
// @Tags SCIM
// @Accept json
// @Produce json
// @Param body body object true "SCIM Group resource with displayName and optional members"
// @Success 201 {object} SCIMGroup
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 409 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups [post]
func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Schemas     []string     `json:"schemas"`
		DisplayName string       `json:"displayName"`
		Members     []SCIMMember `json:"members,omitempty"`
	}
	if err := parseSCIMBody(r, &body); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.DisplayName == "" {
		respondSCIMError(w, http.StatusBadRequest, "displayName is required")
		return
	}

	var (
		id        uuid.UUID
		createdAt time.Time
	)
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO groups (name) VALUES ($1)
		 RETURNING id, created_at`, body.DisplayName,
	).Scan(&id, &createdAt)
	if err != nil {
		h.logger.Error("scim: creating group", "error", err, "name", body.DisplayName)
		respondSCIMError(w, http.StatusConflict, "group already exists or invalid data")
		return
	}

	// Add members if provided.
	for _, m := range body.Members {
		memberID, err := uuid.Parse(m.Value)
		if err != nil {
			h.logger.Warn("scim: invalid member ID in create group", "value", m.Value)
			continue
		}
		_, err = h.pool.Exec(r.Context(),
			`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			memberID, id)
		if err != nil {
			h.logger.Error("scim: adding member to group", "error", err, "user_id", memberID, "group_id", id)
		}
	}

	// Reload members to get display names.
	members, err := h.loadGroupMembers(r.Context(), id)
	if err != nil {
		h.logger.Error("scim: loading members after create", "error", err, "group_id", id)
		members = []SCIMMember{}
	}

	h.logger.Info("scim: group created", "id", id, "name", body.DisplayName)
	respondSCIM(w, http.StatusCreated, dbGroupToSCIM(id, body.DisplayName, createdAt, members))
}

// @Summary Get SCIM group
// @Description Retrieve a single SCIM 2.0 Group resource by ID with members.
// @Tags SCIM
// @Produce json
// @Param id path string true "Group UUID"
// @Success 200 {object} SCIMGroup
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups/{id} [get]
func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid group ID")
		return
	}

	var (
		name      string
		createdAt time.Time
	)
	err = h.pool.QueryRow(r.Context(),
		`SELECT name, created_at FROM groups WHERE id = $1`, id,
	).Scan(&name, &createdAt)
	if err != nil {
		respondSCIMError(w, http.StatusNotFound, "group not found")
		return
	}

	members, err := h.loadGroupMembers(r.Context(), id)
	if err != nil {
		h.logger.Error("scim: loading group members", "error", err, "group_id", id)
		respondSCIMError(w, http.StatusInternalServerError, "failed to load group members")
		return
	}

	respondSCIM(w, http.StatusOK, dbGroupToSCIM(id, name, createdAt, members))
}

// @Summary Replace SCIM group
// @Description Fully replace a SCIM 2.0 Group resource including its member list.
// @Tags SCIM
// @Accept json
// @Produce json
// @Param id path string true "Group UUID"
// @Param body body object true "SCIM Group resource with displayName and members"
// @Success 200 {object} SCIMGroup
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups/{id} [put]
func (h *Handler) replaceGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid group ID")
		return
	}

	var body struct {
		Schemas     []string     `json:"schemas"`
		DisplayName string       `json:"displayName"`
		Members     []SCIMMember `json:"members,omitempty"`
	}
	if err := parseSCIMBody(r, &body); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.DisplayName == "" {
		respondSCIMError(w, http.StatusBadRequest, "displayName is required")
		return
	}

	var createdAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`UPDATE groups SET name = $2 WHERE id = $1 RETURNING created_at`,
		id, body.DisplayName,
	).Scan(&createdAt)
	if err != nil {
		respondSCIMError(w, http.StatusNotFound, "group not found")
		return
	}

	// Replace all members: delete existing, then insert new ones.
	_, err = h.pool.Exec(r.Context(),
		`DELETE FROM user_groups WHERE group_id = $1`, id)
	if err != nil {
		h.logger.Error("scim: clearing group members", "error", err, "group_id", id)
		respondSCIMError(w, http.StatusInternalServerError, "failed to update group members")
		return
	}

	for _, m := range body.Members {
		memberID, err := uuid.Parse(m.Value)
		if err != nil {
			h.logger.Warn("scim: invalid member ID in replace group", "value", m.Value)
			continue
		}
		_, err = h.pool.Exec(r.Context(),
			`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			memberID, id)
		if err != nil {
			h.logger.Error("scim: adding member to group", "error", err, "user_id", memberID, "group_id", id)
		}
	}

	// Reload members to get display names.
	members, err := h.loadGroupMembers(r.Context(), id)
	if err != nil {
		h.logger.Error("scim: loading members after replace", "error", err, "group_id", id)
		members = []SCIMMember{}
	}

	h.logger.Info("scim: group replaced", "id", id, "name", body.DisplayName)
	respondSCIM(w, http.StatusOK, dbGroupToSCIM(id, body.DisplayName, createdAt, members))
}

// @Summary Patch SCIM group
// @Description Apply partial updates to a SCIM 2.0 Group resource (add/remove/replace members or displayName).
// @Tags SCIM
// @Accept json
// @Produce json
// @Param id path string true "Group UUID"
// @Param body body SCIMPatchOp true "SCIM PatchOp request"
// @Success 200 {object} SCIMGroup
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups/{id} [patch]
func (h *Handler) patchGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid group ID")
		return
	}

	// Verify group exists.
	var exists bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM groups WHERE id = $1)`, id,
	).Scan(&exists)
	if err != nil || !exists {
		respondSCIMError(w, http.StatusNotFound, "group not found")
		return
	}

	var patch SCIMPatchOp
	if err := parseSCIMBody(r, &patch); err != nil {
		respondSCIMError(w, http.StatusBadRequest, err.Error())
		return
	}

	for _, op := range patch.Operations {
		switch op.Op {
		case "replace", "Replace":
			if err := h.applyGroupPatch(r.Context(), id, op); err != nil {
				h.logger.Error("scim: group patch replace failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "add", "Add":
			if err := h.applyGroupAdd(r.Context(), id, op); err != nil {
				h.logger.Error("scim: group patch add failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "remove", "Remove":
			if err := h.applyGroupRemove(r.Context(), id, op); err != nil {
				h.logger.Error("scim: group patch remove failed", "error", err, "id", id, "path", op.Path)
				respondSCIMError(w, http.StatusBadRequest, err.Error())
				return
			}
		default:
			respondSCIMError(w, http.StatusBadRequest, fmt.Sprintf("unsupported operation: %s", op.Op))
			return
		}
	}

	h.logger.Info("scim: group patched", "id", id)

	// Return the updated group.
	h.getGroup(w, r)
}

// applyGroupPatch handles replace operations on a group.
func (h *Handler) applyGroupPatch(ctx context.Context, id uuid.UUID, op PatchOperation) error {
	switch op.Path {
	case "displayName":
		val, ok := op.Value.(string)
		if !ok {
			return fmt.Errorf("invalid value for displayName: expected string")
		}
		_, err := h.pool.Exec(ctx,
			`UPDATE groups SET name = $2 WHERE id = $1`, id, val)
		return err

	case "members":
		// Replace replaces all members.
		members, err := patchValueToMembers(op.Value)
		if err != nil {
			return err
		}
		_, err = h.pool.Exec(ctx, `DELETE FROM user_groups WHERE group_id = $1`, id)
		if err != nil {
			return fmt.Errorf("failed to clear members: %w", err)
		}
		for _, m := range members {
			memberID, err := uuid.Parse(m.Value)
			if err != nil {
				continue
			}
			_, err = h.pool.Exec(ctx,
				`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				memberID, id)
			if err != nil {
				return fmt.Errorf("failed to add member %s: %w", m.Value, err)
			}
		}
		return nil

	case "":
		// No path — the value is a map of attributes to set.
		valueMap, ok := op.Value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object value when path is empty")
		}
		for key, val := range valueMap {
			subOp := PatchOperation{Op: op.Op, Path: key, Value: val}
			if err := h.applyGroupPatch(ctx, id, subOp); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported patch path: %s", op.Path)
	}
}

// applyGroupAdd handles add operations on a group (add members).
func (h *Handler) applyGroupAdd(ctx context.Context, id uuid.UUID, op PatchOperation) error {
	switch op.Path {
	case "members":
		members, err := patchValueToMembers(op.Value)
		if err != nil {
			return err
		}
		for _, m := range members {
			memberID, err := uuid.Parse(m.Value)
			if err != nil {
				continue
			}
			_, err = h.pool.Exec(ctx,
				`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				memberID, id)
			if err != nil {
				return fmt.Errorf("failed to add member %s: %w", m.Value, err)
			}
		}
		return nil

	case "displayName":
		return h.applyGroupPatch(ctx, id, op)

	case "":
		valueMap, ok := op.Value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object value when path is empty")
		}
		for key, val := range valueMap {
			subOp := PatchOperation{Op: op.Op, Path: key, Value: val}
			if err := h.applyGroupAdd(ctx, id, subOp); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported patch path: %s", op.Path)
	}
}

// applyGroupRemove handles remove operations on a group (remove members).
func (h *Handler) applyGroupRemove(ctx context.Context, id uuid.UUID, op PatchOperation) error {
	// Handle "members[value eq \"<uuid>\"]" filter syntax used by Azure AD / Okta.
	if memberID, ok := parseMemberFilter(op.Path); ok {
		uid, err := uuid.Parse(memberID)
		if err != nil {
			return fmt.Errorf("invalid member ID in filter: %s", memberID)
		}
		_, err = h.pool.Exec(ctx,
			`DELETE FROM user_groups WHERE user_id = $1 AND group_id = $2`, uid, id)
		return err
	}

	switch op.Path {
	case "members":
		// If value is provided, remove specific members; otherwise remove all.
		if op.Value == nil {
			_, err := h.pool.Exec(ctx, `DELETE FROM user_groups WHERE group_id = $1`, id)
			return err
		}
		members, err := patchValueToMembers(op.Value)
		if err != nil {
			return err
		}
		for _, m := range members {
			memberID, err := uuid.Parse(m.Value)
			if err != nil {
				continue
			}
			_, err = h.pool.Exec(ctx,
				`DELETE FROM user_groups WHERE user_id = $1 AND group_id = $2`,
				memberID, id)
			if err != nil {
				return fmt.Errorf("failed to remove member %s: %w", m.Value, err)
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported remove path: %s", op.Path)
	}
}

// parseMemberFilter parses SCIM filter path like: members[value eq "uuid"]
// and returns the UUID string and true, or empty string and false if not a match.
func parseMemberFilter(path string) (string, bool) {
	// Expected format: members[value eq "some-uuid"]
	const prefix = `members[value eq "`
	const suffix = `"]`
	if len(path) > len(prefix)+len(suffix) &&
		path[:len(prefix)] == prefix &&
		path[len(path)-len(suffix):] == suffix {
		return path[len(prefix) : len(path)-len(suffix)], true
	}
	return "", false
}

// patchValueToMembers converts a PATCH value to a slice of SCIMMember.
func patchValueToMembers(value any) ([]SCIMMember, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid members value: %w", err)
	}
	var members []SCIMMember
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("invalid members format: %w", err)
	}
	return members, nil
}

// @Summary Delete SCIM group
// @Description Permanently delete a SCIM 2.0 Group resource and remove all member associations.
// @Tags SCIM
// @Param id path string true "Group UUID"
// @Success 204 "No Content"
// @Failure 400 {object} SCIMError
// @Failure 401 {object} SCIMError
// @Failure 404 {object} SCIMError
// @Failure 500 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Groups/{id} [delete]
func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondSCIMError(w, http.StatusBadRequest, "invalid group ID")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		h.logger.Error("scim: deleting group", "error", err, "id", id)
		respondSCIMError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}
	if tag.RowsAffected() == 0 {
		respondSCIMError(w, http.StatusNotFound, "group not found")
		return
	}

	h.logger.Info("scim: group deleted", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// --- Discovery Endpoints ---

// @Summary Get SCIM ServiceProviderConfig
// @Description Return the SCIM 2.0 Service Provider Configuration (RFC 7643 section 5).
// @Tags SCIM
// @Produce json
// @Success 200 {object} object
// @Failure 401 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/ServiceProviderConfig [get]
func (h *Handler) serviceProviderConfig(w http.ResponseWriter, r *http.Request) {
	_ = r
	respondSCIM(w, http.StatusOK, map[string]any{
		"schemas": []string{schemaServiceProviderCfg},
		"documentationUri": "https://outpost.example.com/docs/scim",
		"patch": map[string]any{
			"supported": true,
		},
		"bulk": map[string]any{
			"supported":  false,
			"maxOperations": 0,
			"maxPayloadSize": 0,
		},
		"filter": map[string]any{
			"supported":  true,
			"maxResults": 200,
		},
		"changePassword": map[string]any{
			"supported": false,
		},
		"sort": map[string]any{
			"supported": false,
		},
		"etag": map[string]any{
			"supported": false,
		},
		"authenticationSchemes": []map[string]any{
			{
				"name":        "OAuth Bearer Token",
				"description": "Authentication scheme using the OAuth Bearer Token Standard",
				"specUri":     "https://www.rfc-editor.org/info/rfc6750",
				"type":        "oauthbearertoken",
				"primary":     true,
			},
		},
	})
}

// @Summary Get SCIM Schemas
// @Description Return the SCIM 2.0 schema definitions for User and Group resources.
// @Tags SCIM
// @Produce json
// @Success 200 {object} SCIMListResponse
// @Failure 401 {object} SCIMError
// @Security SCIMBearerToken
// @Router /scim/v2/Schemas [get]
func (h *Handler) schemas(w http.ResponseWriter, r *http.Request) {
	_ = r
	respondSCIM(w, http.StatusOK, SCIMListResponse{
		Schemas:      []string{schemaListResponse},
		TotalResults: 2,
		StartIndex:   1,
		ItemsPerPage: 2,
		Resources: []map[string]any{
			{
				"schemas":     []string{schemaSchema},
				"id":          schemaUser,
				"name":        "User",
				"description": "User Account",
				"attributes": []map[string]any{
					{"name": "userName", "type": "string", "multiValued": false, "required": true, "uniqueness": "server"},
					{"name": "name", "type": "complex", "multiValued": false, "required": false,
						"subAttributes": []map[string]any{
							{"name": "givenName", "type": "string", "multiValued": false},
							{"name": "familyName", "type": "string", "multiValued": false},
							{"name": "formatted", "type": "string", "multiValued": false},
						},
					},
					{"name": "emails", "type": "complex", "multiValued": true, "required": false,
						"subAttributes": []map[string]any{
							{"name": "value", "type": "string", "multiValued": false},
							{"name": "type", "type": "string", "multiValued": false},
							{"name": "primary", "type": "boolean", "multiValued": false},
						},
					},
					{"name": "active", "type": "boolean", "multiValued": false, "required": false},
					{"name": "externalId", "type": "string", "multiValued": false, "required": false},
				},
			},
			{
				"schemas":     []string{schemaSchema},
				"id":          schemaGroup,
				"name":        "Group",
				"description": "Group",
				"attributes": []map[string]any{
					{"name": "displayName", "type": "string", "multiValued": false, "required": true},
					{"name": "members", "type": "complex", "multiValued": true, "required": false,
						"subAttributes": []map[string]any{
							{"name": "value", "type": "string", "multiValued": false},
							{"name": "display", "type": "string", "multiValued": false},
							{"name": "$ref", "type": "reference", "multiValued": false},
						},
					},
				},
			},
		},
	})
}

// strPtrOrNil returns a pointer to s if non-empty, or nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nilIfEmpty returns nil if the string pointer is nil or empty, otherwise
// returns the pointer as-is.
func nilIfEmpty(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}
