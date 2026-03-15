package mfa

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebAuthnCredential represents a stored WebAuthn/FIDO2 credential.
type WebAuthnCredential struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	CredentialID []byte    `json:"credential_id"`
	PublicKey    []byte    `json:"public_key"`
	SignCount    int64     `json:"sign_count"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

// WebAuthnStore provides storage operations for WebAuthn credentials.
// It does not implement the full WebAuthn ceremony; that will be handled
// by a WebAuthn library and the frontend.
type WebAuthnStore struct {
	pool *pgxpool.Pool
}

// NewWebAuthnStore creates a new WebAuthnStore.
func NewWebAuthnStore(pool *pgxpool.Pool) *WebAuthnStore {
	return &WebAuthnStore{pool: pool}
}

// RegisterCredential stores a new WebAuthn credential in the database.
func (s *WebAuthnStore) RegisterCredential(ctx context.Context, cred WebAuthnCredential) error {
	if cred.ID == "" {
		cred.ID = uuid.New().String()
	}
	if cred.CreatedAt.IsZero() {
		cred.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO mfa_webauthn (id, user_id, credential_id, public_key, sign_count, name, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		cred.ID, cred.UserID, cred.CredentialID, cred.PublicKey,
		cred.SignCount, cred.Name, cred.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting WebAuthn credential: %w", err)
	}
	return nil
}

// GetCredentials returns all WebAuthn credentials for a user.
func (s *WebAuthnStore) GetCredentials(ctx context.Context, userID string) ([]WebAuthnCredential, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, credential_id, public_key, sign_count, name, created_at
		 FROM mfa_webauthn WHERE user_id = $1 ORDER BY created_at`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying WebAuthn credentials: %w", err)
	}
	defer rows.Close()

	var creds []WebAuthnCredential
	for rows.Next() {
		var c WebAuthnCredential
		if err := rows.Scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey,
			&c.SignCount, &c.Name, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning WebAuthn credential: %w", err)
		}
		creds = append(creds, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating WebAuthn credentials: %w", err)
	}
	return creds, nil
}

// UpdateSignCount updates the signature counter for a credential after a
// successful authentication ceremony.
func (s *WebAuthnStore) UpdateSignCount(ctx context.Context, credentialID string, newCount int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mfa_webauthn SET sign_count = $1 WHERE id = $2`,
		newCount, credentialID,
	)
	if err != nil {
		return fmt.Errorf("updating sign count: %w", err)
	}
	return nil
}

// DeleteCredential removes a WebAuthn credential by its database ID.
func (s *WebAuthnStore) DeleteCredential(ctx context.Context, credentialID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM mfa_webauthn WHERE id = $1`, credentialID,
	)
	if err != nil {
		return fmt.Errorf("deleting WebAuthn credential: %w", err)
	}
	return nil
}

// DeleteCredentialForUser removes a WebAuthn credential only if it belongs
// to the specified user, preventing cross-user deletion.
func (s *WebAuthnStore) DeleteCredentialForUser(ctx context.Context, credentialID, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM mfa_webauthn WHERE id = $1 AND user_id = $2`, credentialID, userID,
	)
	if err != nil {
		return fmt.Errorf("deleting WebAuthn credential: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("credential not found or not owned by user")
	}
	return nil
}
