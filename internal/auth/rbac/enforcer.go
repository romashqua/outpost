package rbac

import "sync"

// Permission is a "resource:action" string (e.g. "user:read", "network:write", "*").
type Permission = string

// Role groups a set of permissions under a name.
type Role struct {
	Name        string
	Permissions []Permission
}

// Enforcer evaluates access control decisions using an in-memory role/permission cache.
type Enforcer struct {
	mu    sync.RWMutex
	roles map[string]map[string]bool // role name -> set of permissions
}

// NewEnforcer creates a ready-to-use Enforcer.
func NewEnforcer() *Enforcer {
	return &Enforcer{
		roles: make(map[string]map[string]bool),
	}
}

// LoadRoles replaces the current role definitions with the provided set.
func (e *Enforcer) LoadRoles(roles []Role) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.roles = make(map[string]map[string]bool, len(roles))
	for _, r := range roles {
		perms := make(map[string]bool, len(r.Permissions))
		for _, p := range r.Permissions {
			perms[p] = true
		}
		e.roles[r.Name] = perms
	}
}

// Can returns true if any of the user's roles grant access to the given
// resource and action. The wildcard permission "*" grants access to everything.
func (e *Enforcer) Can(userRoles []string, resource, action string) bool {
	required := resource + ":" + action

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, roleName := range userRoles {
		perms, ok := e.roles[roleName]
		if !ok {
			continue
		}
		if perms["*"] {
			return true
		}
		if perms[required] {
			return true
		}
	}
	return false
}
