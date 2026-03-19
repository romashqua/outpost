package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/romashqua/outpost/internal/auth"
)

// FirewallRefresher pushes updated firewall configs to gateways when ACL/group membership changes.
type FirewallRefresher interface {
	RefreshFirewallForUser(userID string)
	RefreshFirewallForGroup(groupID string)
}

// GroupHandler provides CRUD endpoints for group management and ACL assignment.
type GroupHandler struct {
	pool DB
	log       *slog.Logger
	refresher FirewallRefresher
}

// NewGroupHandler creates a GroupHandler backed by the given connection pool.
func NewGroupHandler(pool DB, logger ...*slog.Logger) *GroupHandler {
	l := slog.Default()
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	}
	return &GroupHandler{pool: pool, log: l.With("handler", "group")}
}

// WithFirewallRefresher sets the firewall refresher for pushing ACL changes to gateways.
func (h *GroupHandler) WithFirewallRefresher(r FirewallRefresher) *GroupHandler {
	h.refresher = r
	return h
}

// Routes returns a chi.Router with group CRUD and ACL endpoints mounted.
func (h *GroupHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.With(auth.RequireAdmin).Post("/", h.create)
	r.Route("/{id}", func(r chi.Router) {
		r.Get("/", h.get)
		r.With(auth.RequireAdmin).Put("/", h.update)
		r.With(auth.RequireAdmin).Delete("/", h.delete)
		r.Get("/members", h.listMembers)
		r.With(auth.RequireAdmin).Post("/members", h.addMember)
		r.With(auth.RequireAdmin).Delete("/members/{userId}", h.removeMember)
		r.Get("/acls", h.listACLs)
		r.With(auth.RequireAdmin).Post("/acls", h.addACL)
		r.With(auth.RequireAdmin).Delete("/acls/{aclId}", h.removeACL)
	})
	return r
}

// --- Types ---

type groupListItem struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	MemberCount int       `json:"member_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type groupMember struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Email    string    `json:"email"`
}

type groupACL struct {
	ID          uuid.UUID `json:"id"`
	NetworkID   uuid.UUID `json:"network_id"`
	NetworkName string    `json:"network_name"`
	AllowedIPs  []string  `json:"allowed_ips"`
}

type groupDetail struct {
	ID          uuid.UUID     `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	IsSystem    bool          `json:"is_system"`
	CreatedAt   time.Time     `json:"created_at"`
	Members     []groupMember `json:"members"`
	ACLs        []groupACL    `json:"acls"`
}

type createGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// --- Handlers ---

// @Summary List groups
// @Description Returns all groups with member counts.
// @Tags Groups
// @Produce json
// @Success 200 {array} groupListItem
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups [get]
func (h *GroupHandler) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT g.id, g.name, g.description, g.is_system, g.created_at,
		        COUNT(ug.user_id)::int AS member_count
		 FROM groups g
		 LEFT JOIN user_groups ug ON ug.group_id = g.id
		 GROUP BY g.id
		 ORDER BY g.created_at DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query groups")
		return
	}
	defer rows.Close()

	groups := make([]groupListItem, 0)
	for rows.Next() {
		var g groupListItem
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.IsSystem, &g.CreatedAt, &g.MemberCount); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan group")
			return
		}
		groups = append(groups, g)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate groups")
		return
	}

	respondJSON(w, http.StatusOK, groups)
}

// @Summary Create group
// @Description Create a new group. Requires admin privileges.
// @Tags Groups
// @Accept json
// @Produce json
// @Param body body createGroupRequest true "Group data"
// @Success 201 {object} groupListItem
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups [post]
func (h *GroupHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	var g groupListItem
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO groups (name, description)
		 VALUES ($1, $2)
		 RETURNING id, name, description, is_system, created_at`,
		req.Name, req.Description,
	).Scan(&g.ID, &g.Name, &g.Description, &g.IsSystem, &g.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			respondError(w, http.StatusConflict, "group with this name already exists")
			return
		}
		h.log.Error("failed to create group", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create group")
		return
	}

	g.MemberCount = 0
	h.log.Info("group created", "id", g.ID, "name", g.Name)
	respondJSON(w, http.StatusCreated, g)
}

// @Summary Get group
// @Description Retrieve a group by ID with its members and ACLs.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Success 200 {object} groupDetail
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id} [get]
func (h *GroupHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var g groupDetail
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, description, is_system, created_at
		 FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.Name, &g.Description, &g.IsSystem, &g.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "group not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch group")
		}
		return
	}

	// Fetch members.
	g.Members = make([]groupMember, 0)
	memberRows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.username, u.email
		 FROM users u
		 JOIN user_groups ug ON ug.user_id = u.id
		 WHERE ug.group_id = $1
		 ORDER BY u.username`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query members")
		return
	}
	defer memberRows.Close()

	for memberRows.Next() {
		var m groupMember
		if err := memberRows.Scan(&m.UserID, &m.Username, &m.Email); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		g.Members = append(g.Members, m)
	}
	if err := memberRows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate members")
		return
	}

	// Fetch ACLs.
	g.ACLs = make([]groupACL, 0)
	aclRows, err := h.pool.Query(r.Context(),
		`SELECT a.id, a.network_id, n.name,
		        ARRAY(SELECT unnest(a.allowed_ips)::text)
		 FROM network_acls a
		 JOIN networks n ON n.id = a.network_id
		 WHERE a.group_id = $1
		 ORDER BY n.name`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query acls")
		return
	}
	defer aclRows.Close()

	for aclRows.Next() {
		var acl groupACL
		if err := aclRows.Scan(&acl.ID, &acl.NetworkID, &acl.NetworkName, &acl.AllowedIPs); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan acl")
			return
		}
		if acl.AllowedIPs == nil {
			acl.AllowedIPs = make([]string, 0)
		}
		g.ACLs = append(g.ACLs, acl)
	}
	if err := aclRows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate acls")
		return
	}

	respondJSON(w, http.StatusOK, g)
}

// @Summary Update group
// @Description Update an existing group. Requires admin privileges.
// @Tags Groups
// @Accept json
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Param body body updateGroupRequest true "Fields to update"
// @Success 200 {object} groupListItem
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id} [put]
func (h *GroupHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateGroupRequest
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var g groupListItem
	err = h.pool.QueryRow(r.Context(),
		`UPDATE groups SET
			name        = COALESCE($2, name),
			description = COALESCE($3, description)
		 WHERE id = $1
		 RETURNING id, name, description, is_system, created_at,
		           (SELECT COUNT(*) FROM user_groups WHERE group_id = $1)`,
		id, req.Name, req.Description,
	).Scan(&g.ID, &g.Name, &g.Description, &g.IsSystem, &g.CreatedAt, &g.MemberCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "group not found")
		} else {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				respondError(w, http.StatusConflict, "group with this name already exists")
			} else {
				respondError(w, http.StatusInternalServerError, "failed to update group")
			}
		}
		return
	}

	respondJSON(w, http.StatusOK, g)
}

// @Summary Delete group
// @Description Delete a group by ID. System groups cannot be deleted. Requires admin privileges.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id} [delete]
func (h *GroupHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Prevent deleting system groups.
	var isSystem bool
	err = h.pool.QueryRow(r.Context(),
		`SELECT is_system FROM groups WHERE id = $1`, id,
	).Scan(&isSystem)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "group not found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to fetch group")
		}
		return
	}
	if isSystem {
		respondError(w, http.StatusForbidden, "cannot delete a system group")
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete group")
		return
	}

	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "group not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Members ---

// @Summary List group members
// @Description Returns all members of a group.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Success 200 {array} groupMember
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/members [get]
func (h *GroupHandler) listMembers(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid group id")
		return
	}

	members := make([]groupMember, 0)
	rows, err := h.pool.Query(r.Context(),
		`SELECT u.id, u.username, u.email
		 FROM users u
		 JOIN user_groups ug ON ug.user_id = u.id
		 WHERE ug.group_id = $1
		 ORDER BY u.username`, groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query members")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var m groupMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.Email); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate members")
		return
	}

	respondJSON(w, http.StatusOK, members)
}

// @Summary Add group member
// @Description Add a user to a group. Requires admin privileges.
// @Tags Groups
// @Accept json
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Param body body object true "User ID to add" example({"user_id": "uuid"})
// @Success 201 "Created"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/members [post]
func (h *GroupHandler) addMember(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.UserID == "" {
		respondError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO user_groups (user_id, group_id) VALUES ($1, $2)
		 ON CONFLICT (user_id, group_id) DO NOTHING`,
		userID, groupID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			respondError(w, http.StatusBadRequest, "user or group not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	h.log.Info("member added to group", "group_id", groupID, "user_id", userID)
	if h.refresher != nil {
		h.refresher.RefreshFirewallForUser(userID.String())
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "added", "user_id": userID.String(), "group_id": groupID.String()})
}

// @Summary Remove group member
// @Description Remove a user from a group. Requires admin privileges.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Param userId path string true "User ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/members/{userId} [delete]
func (h *GroupHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, err := parseUUID(r, "userId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM user_groups WHERE user_id = $1 AND group_id = $2`,
		userID, groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to remove member")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "member not found")
		return
	}

	if h.refresher != nil {
		h.refresher.RefreshFirewallForUser(userID.String())
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- ACLs ---

// @Summary List group ACLs
// @Description Returns all network ACLs for a group.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Success 200 {array} groupACL
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/acls [get]
func (h *GroupHandler) listACLs(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT a.id, a.network_id, n.name,
		        ARRAY(SELECT unnest(a.allowed_ips)::text)
		 FROM network_acls a
		 JOIN networks n ON n.id = a.network_id
		 WHERE a.group_id = $1
		 ORDER BY n.name`, groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query acls")
		return
	}
	defer rows.Close()

	acls := make([]groupACL, 0)
	for rows.Next() {
		var acl groupACL
		if err := rows.Scan(&acl.ID, &acl.NetworkID, &acl.NetworkName, &acl.AllowedIPs); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan acl")
			return
		}
		if acl.AllowedIPs == nil {
			acl.AllowedIPs = make([]string, 0)
		}
		acls = append(acls, acl)
	}

	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to iterate acls")
		return
	}

	respondJSON(w, http.StatusOK, acls)
}

// @Summary Add group ACL
// @Description Add a network ACL to a group. Requires admin privileges.
// @Tags Groups
// @Accept json
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Param body body object true "Network ID and allowed IPs"
// @Success 201 {object} groupACL
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/acls [post]
func (h *GroupHandler) addACL(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		NetworkID  string   `json:"network_id"`
		AllowedIPs []string `json:"allowed_ips"`
	}
	if err := parseBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.NetworkID == "" {
		respondError(w, http.StatusBadRequest, "network_id is required")
		return
	}

	networkID, err := uuid.Parse(req.NetworkID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid network_id")
		return
	}

	allowedIPs := req.AllowedIPs
	if allowedIPs == nil {
		allowedIPs = []string{"0.0.0.0/0"}
	}

	var acl groupACL
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO network_acls (network_id, group_id, allowed_ips)
		 VALUES ($1, $2, $3::cidr[])
		 RETURNING id, network_id, (SELECT name FROM networks WHERE id = $1), allowed_ips::text[]`,
		networkID, groupID, allowedIPs,
	).Scan(&acl.ID, &acl.NetworkID, &acl.NetworkName, &acl.AllowedIPs)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" {
				respondError(w, http.StatusConflict, "acl for this network and group already exists")
				return
			}
			if pgErr.Code == "23503" {
				respondError(w, http.StatusBadRequest, "network or group not found")
				return
			}
			if pgErr.Code == "22P02" {
				respondError(w, http.StatusBadRequest, "invalid CIDR format in allowed_ips")
				return
			}
		}
		h.log.Error("failed to create acl", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create acl")
		return
	}

	if acl.AllowedIPs == nil {
		acl.AllowedIPs = make([]string, 0)
	}

	h.log.Info("acl added", "group_id", groupID, "network_id", networkID)
	if h.refresher != nil {
		h.refresher.RefreshFirewallForGroup(groupID.String())
	}
	respondJSON(w, http.StatusCreated, acl)
}

// @Summary Remove group ACL
// @Description Remove a network ACL from a group. Requires admin privileges.
// @Tags Groups
// @Produce json
// @Param id path string true "Group ID (UUID)"
// @Param aclId path string true "ACL ID (UUID)"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /groups/{id}/acls/{aclId} [delete]
func (h *GroupHandler) removeACL(w http.ResponseWriter, r *http.Request) {
	groupID, err := parseUUID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	aclID, err := parseUUID(r, "aclId")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM network_acls WHERE id = $1 AND group_id = $2`,
		aclID, groupID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete acl")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "acl not found")
		return
	}

	if h.refresher != nil {
		h.refresher.RefreshFirewallForGroup(groupID.String())
	}
	w.WriteHeader(http.StatusNoContent)
}
