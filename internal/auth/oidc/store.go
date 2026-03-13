package oidc

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client represents an OAuth2/OIDC client registration.
type Client struct {
	ID           string
	SecretHash   string
	Name         string
	RedirectURIs []string
	Scopes       []string
	CreatedAt    time.Time
}

// AuthCode represents a stored authorization code.
type AuthCode struct {
	Code                string
	ClientID            string
	UserID              string
	Scopes              []string
	Nonce               string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
	CreatedAt           time.Time
}

// Store encapsulates database operations for the OIDC provider.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new OIDC store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// GetClient retrieves an OIDC client by its ID.
func (s *Store) GetClient(ctx context.Context, clientID string) (*Client, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, secret_hash, name, redirect_uris, scopes, created_at
		 FROM oidc_clients WHERE id = $1`, clientID)

	var c Client
	err := row.Scan(&c.ID, &c.SecretHash, &c.Name, &c.RedirectURIs, &c.Scopes, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("oidc: get client %q: %w", clientID, err)
	}
	return &c, nil
}

// ValidateRedirectURI checks whether the given URI is registered for the client.
func ValidateRedirectURI(client *Client, uri string) bool {
	for _, allowed := range client.RedirectURIs {
		if allowed == uri {
			return true
		}
	}
	return false
}

// CreateAuthCode persists an authorization code to the database.
func (s *Store) CreateAuthCode(ctx context.Context, code AuthCode) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oidc_auth_codes
		 (code, client_id, user_id, scopes, nonce, redirect_uri,
		  code_challenge, code_challenge_method, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		code.Code, code.ClientID, code.UserID, code.Scopes, code.Nonce,
		code.RedirectURI, code.CodeChallenge, code.CodeChallengeMethod,
		code.ExpiresAt, code.CreatedAt)
	if err != nil {
		return fmt.Errorf("oidc: create auth code: %w", err)
	}
	return nil
}

// ConsumeAuthCode retrieves and deletes an authorization code in a single
// atomic operation, ensuring single-use semantics.
func (s *Store) ConsumeAuthCode(ctx context.Context, code string) (*AuthCode, error) {
	row := s.pool.QueryRow(ctx,
		`DELETE FROM oidc_auth_codes WHERE code = $1
		 RETURNING code, client_id, user_id, scopes, nonce, redirect_uri,
		           code_challenge, code_challenge_method, expires_at, created_at`,
		code)

	var ac AuthCode
	err := row.Scan(
		&ac.Code, &ac.ClientID, &ac.UserID, &ac.Scopes, &ac.Nonce,
		&ac.RedirectURI, &ac.CodeChallenge, &ac.CodeChallengeMethod,
		&ac.ExpiresAt, &ac.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("oidc: consume auth code: %w", err)
	}
	return &ac, nil
}

// GetUserInfo retrieves basic user information for building OIDC claims.
// This queries the users table which is expected to have at minimum:
// id, email, email_verified, full_name, username columns.
func (s *Store) GetUserInfo(ctx context.Context, userID string) (*UserInfo, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, email, email_verified, full_name, username
		 FROM users WHERE id = $1`, userID)

	var u UserInfo
	err := row.Scan(&u.UserID, &u.Email, &u.EmailVerified, &u.Name, &u.Username)
	if err != nil {
		return nil, fmt.Errorf("oidc: get user %q: %w", userID, err)
	}
	return &u, nil
}

// GetUserGroups retrieves group names for a user.
func (s *Store) GetUserGroups(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.name FROM groups g
		 JOIN user_groups ug ON ug.group_id = g.id
		 WHERE ug.user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("oidc: get user groups: %w", err)
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("oidc: scan group: %w", err)
		}
		groups = append(groups, name)
	}
	return groups, rows.Err()
}
