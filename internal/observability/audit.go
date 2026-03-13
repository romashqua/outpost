package observability

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditLogger writes audit events to PostgreSQL.
type AuditLogger struct {
	pool *pgxpool.Pool
}

// NewAuditLogger creates an AuditLogger backed by the given connection pool.
func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

// Log records an audit event.
func (a *AuditLogger) Log(
	ctx context.Context,
	userID uuid.UUID,
	action string,
	resource string,
	details map[string]any,
	ipAddress string,
	userAgent string,
) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal audit details: %w", err)
	}

	const query = `
		INSERT INTO audit_log (user_id, action, resource, details, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
	`

	_, err = a.pool.Exec(ctx, query, userID, action, resource, detailsJSON, ipAddress, userAgent)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}
