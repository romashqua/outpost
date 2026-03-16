package rbac

import "testing"

func TestNewEnforcer(t *testing.T) {
	e := NewEnforcer()
	if e == nil {
		t.Fatal("expected non-nil enforcer")
	}
	// No roles loaded: any check should fail.
	if e.Can([]string{"admin"}, "user", "read") {
		t.Error("expected false with no roles loaded")
	}
}

func TestEnforcer_LoadRoles_And_Can(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "admin", Permissions: []Permission{"*"}},
		{Name: "viewer", Permissions: []Permission{"user:read", "network:read"}},
		{Name: "editor", Permissions: []Permission{"user:read", "user:write", "network:read", "network:write"}},
	})

	tests := []struct {
		roles    []string
		resource string
		action   string
		want     bool
		desc     string
	}{
		{[]string{"admin"}, "user", "read", true, "admin wildcard grants user:read"},
		{[]string{"admin"}, "anything", "delete", true, "admin wildcard grants anything"},
		{[]string{"viewer"}, "user", "read", true, "viewer has user:read"},
		{[]string{"viewer"}, "user", "write", false, "viewer lacks user:write"},
		{[]string{"viewer"}, "network", "read", true, "viewer has network:read"},
		{[]string{"viewer"}, "network", "write", false, "viewer lacks network:write"},
		{[]string{"editor"}, "user", "write", true, "editor has user:write"},
		{[]string{"editor"}, "device", "read", false, "editor lacks device:read"},
		{[]string{"viewer", "editor"}, "user", "write", true, "multi-role: editor grants user:write"},
		{[]string{"unknown"}, "user", "read", false, "unknown role has no permissions"},
		{nil, "user", "read", false, "nil roles deny all"},
		{[]string{}, "user", "read", false, "empty roles deny all"},
	}

	for _, tt := range tests {
		got := e.Can(tt.roles, tt.resource, tt.action)
		if got != tt.want {
			t.Errorf("%s: Can(%v, %q, %q) = %v, want %v", tt.desc, tt.roles, tt.resource, tt.action, got, tt.want)
		}
	}
}

func TestEnforcer_LoadRoles_ReplacesExisting(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "admin", Permissions: []Permission{"*"}},
	})
	if !e.Can([]string{"admin"}, "user", "read") {
		t.Fatal("expected admin to have access before reload")
	}

	// Replace with roles that don't include admin.
	e.LoadRoles([]Role{
		{Name: "viewer", Permissions: []Permission{"user:read"}},
	})
	if e.Can([]string{"admin"}, "user", "read") {
		t.Error("expected admin to lose access after roles replaced")
	}
	if !e.Can([]string{"viewer"}, "user", "read") {
		t.Error("expected viewer to have user:read after reload")
	}
}

func TestEnforcer_WildcardPermission(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "superuser", Permissions: []Permission{"*"}},
	})

	// Wildcard should grant access to any resource:action.
	resources := []string{"user", "network", "device", "gateway", "s2s", "setting"}
	actions := []string{"read", "write", "delete", "admin"}
	for _, res := range resources {
		for _, act := range actions {
			if !e.Can([]string{"superuser"}, res, act) {
				t.Errorf("expected wildcard to grant %s:%s", res, act)
			}
		}
	}
}

func TestEnforcer_EmptyRoles(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{})
	if e.Can([]string{"admin"}, "user", "read") {
		t.Error("expected no access with empty role set")
	}
}

func TestEnforcer_RoleWithNoPermissions(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "empty-role", Permissions: nil},
	})
	if e.Can([]string{"empty-role"}, "user", "read") {
		t.Error("expected role with no permissions to deny access")
	}
}

func TestEnforcer_DuplicatePermissions(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "dup", Permissions: []Permission{"user:read", "user:read", "user:read"}},
	})
	if !e.Can([]string{"dup"}, "user", "read") {
		t.Error("expected duplicate permissions to still grant access")
	}
}

func TestEnforcer_PermissionFormatExactMatch(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "test", Permissions: []Permission{"user:read"}},
	})
	// The permission "user:read" should not grant "user:readonly" or "users:read".
	if e.Can([]string{"test"}, "user", "readonly") {
		t.Error("expected exact action match")
	}
	if e.Can([]string{"test"}, "users", "read") {
		t.Error("expected exact resource match")
	}
}

func TestEnforcer_ConcurrentAccess(t *testing.T) {
	e := NewEnforcer()
	e.LoadRoles([]Role{
		{Name: "admin", Permissions: []Permission{"*"}},
		{Name: "viewer", Permissions: []Permission{"user:read"}},
	})

	done := make(chan bool, 100)
	for i := 0; i < 50; i++ {
		go func() {
			_ = e.Can([]string{"admin"}, "user", "read")
			done <- true
		}()
		go func() {
			e.LoadRoles([]Role{
				{Name: "admin", Permissions: []Permission{"*"}},
				{Name: "viewer", Permissions: []Permission{"user:read"}},
			})
			done <- true
		}()
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}
