package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TokenBlacklist checks whether a JWT has been revoked.
type TokenBlacklist interface {
	// IsBlacklisted returns true if the token (identified by raw JWT string) has been revoked.
	IsBlacklisted(ctx context.Context, rawToken string) (bool, error)
	// Add blacklists a token until its natural expiry time.
	Add(ctx context.Context, rawToken string, expiresAt time.Time) error
}

// HashToken returns the hex-encoded SHA-256 hash of a raw JWT string.
func HashToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}

// DBTokenBlacklist is a PostgreSQL-backed TokenBlacklist.
type DBTokenBlacklist struct {
	pool *pgxpool.Pool
}

// NewDBTokenBlacklist creates a new DB-backed blacklist.
func NewDBTokenBlacklist(pool *pgxpool.Pool) *DBTokenBlacklist {
	return &DBTokenBlacklist{pool: pool}
}

// IsBlacklisted checks whether the given raw JWT is in the blacklist.
func (b *DBTokenBlacklist) IsBlacklisted(ctx context.Context, rawToken string) (bool, error) {
	hash := HashToken(rawToken)
	var exists bool
	err := b.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM token_blacklist WHERE token_hash = $1)`,
		hash,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// Add inserts a token hash into the blacklist. The entry will be kept until
// expires_at, after which a periodic cleanup job can remove it.
func (b *DBTokenBlacklist) Add(ctx context.Context, rawToken string, expiresAt time.Time) error {
	hash := HashToken(rawToken)
	_, err := b.pool.Exec(ctx,
		`INSERT INTO token_blacklist (token_hash, expires_at) VALUES ($1, $2)
		 ON CONFLICT (token_hash) DO NOTHING`,
		hash, expiresAt,
	)
	return err
}

// Cleanup deletes expired entries from the token blacklist.
func (b *DBTokenBlacklist) Cleanup(ctx context.Context) error {
	_, err := b.pool.Exec(ctx,
		`DELETE FROM token_blacklist WHERE expires_at < now()`,
	)
	return err
}
