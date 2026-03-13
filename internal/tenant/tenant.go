package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const tenantContextKey contextKey = iota

// Tenant represents an organization/tenant.
type Tenant struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Plan        string    `json:"plan"` // free, pro, enterprise
	MaxUsers    int       `json:"max_users"`
	MaxDevices  int       `json:"max_devices"`
	MaxNetworks int       `json:"max_networks"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FromContext extracts the tenant from the request context, if set.
func FromContext(ctx context.Context) (*Tenant, bool) {
	t, ok := ctx.Value(tenantContextKey).(*Tenant)
	return t, ok
}

// WithTenant returns a new context with the given tenant attached.
func WithTenant(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, tenantContextKey, t)
}

// TenantMiddleware returns HTTP middleware that extracts the tenant from the
// X-Tenant-ID header or the first subdomain of the Host header, then loads
// the tenant from the database and places it in the request context.
func TenantMiddleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	mgr := NewManager(pool)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var t *Tenant
			var err error

			// Check header first.
			if id := r.Header.Get("X-Tenant-ID"); id != "" {
				t, err = mgr.Get(r.Context(), id)
			} else {
				// Fall back to subdomain.
				slug := extractSubdomain(r.Host)
				if slug != "" {
					t, err = mgr.GetBySlug(r.Context(), slug)
				}
			}

			if err != nil {
				writeError(w, http.StatusNotFound, "tenant not found")
				return
			}

			if t != nil {
				if !t.IsActive {
					writeError(w, http.StatusForbidden, "tenant is disabled")
					return
				}
				r = r.WithContext(WithTenant(r.Context(), t))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractSubdomain returns the first subdomain from a host string.
// For example, "acme.outpost.example.com" returns "acme".
func extractSubdomain(host string) string {
	// Strip port if present.
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		return parts[0]
	}
	return ""
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Manager handles tenant CRUD operations.
type Manager struct {
	pool *pgxpool.Pool
}

// NewManager creates a new tenant Manager.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

// Create inserts a new tenant and returns it with generated ID and timestamps.
func (m *Manager) Create(ctx context.Context, t Tenant) (*Tenant, error) {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}

	err := m.pool.QueryRow(ctx,
		`INSERT INTO tenants (id, name, slug, plan, max_users, max_devices, max_networks, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at, updated_at`,
		t.ID, t.Name, t.Slug, t.Plan, t.MaxUsers, t.MaxDevices, t.MaxNetworks, t.IsActive,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert tenant: %w", err)
	}

	return &t, nil
}

// Get retrieves a tenant by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Tenant, error) {
	var t Tenant
	err := m.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, max_users, max_devices, max_networks, is_active, created_at, updated_at
		 FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Plan, &t.MaxUsers, &t.MaxDevices,
		&t.MaxNetworks, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get tenant: %w", err)
	}
	return &t, nil
}

// GetBySlug retrieves a tenant by its URL-friendly slug.
func (m *Manager) GetBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var t Tenant
	err := m.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, max_users, max_devices, max_networks, is_active, created_at, updated_at
		 FROM tenants WHERE slug = $1`, slug,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.Plan, &t.MaxUsers, &t.MaxDevices,
		&t.MaxNetworks, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get tenant by slug: %w", err)
	}
	return &t, nil
}

// List returns all tenants ordered by creation date.
func (m *Manager) List(ctx context.Context) ([]Tenant, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, name, slug, plan, max_users, max_devices, max_networks, is_active, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Plan, &t.MaxUsers, &t.MaxDevices,
			&t.MaxNetworks, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}

	return tenants, nil
}

// Update modifies an existing tenant.
func (m *Manager) Update(ctx context.Context, t Tenant) error {
	tag, err := m.pool.Exec(ctx,
		`UPDATE tenants SET
			name = $2, slug = $3, plan = $4, max_users = $5, max_devices = $6,
			max_networks = $7, is_active = $8, updated_at = now()
		 WHERE id = $1`,
		t.ID, t.Name, t.Slug, t.Plan, t.MaxUsers, t.MaxDevices, t.MaxNetworks, t.IsActive,
	)
	if err != nil {
		return fmt.Errorf("update tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}

// Delete removes a tenant by ID.
func (m *Manager) Delete(ctx context.Context, id string) error {
	tag, err := m.pool.Exec(ctx,
		`DELETE FROM tenants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tenant not found")
	}
	return nil
}
