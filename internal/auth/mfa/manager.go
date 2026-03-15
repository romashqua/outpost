package mfa

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// MFAStatus describes which MFA methods are currently active for a user.
type MFAStatus struct {
	MFAEnabled       bool `json:"mfa_enabled"`
	TOTPConfigured   bool `json:"totp_configured"`
	TOTPVerified     bool `json:"totp_verified"`
	WebAuthnCount    int  `json:"webauthn_count"`
	BackupCodesLeft  int  `json:"backup_codes_left"`
}

// Manager is the unified MFA manager coordinating TOTP, WebAuthn, backup
// codes, and the users.mfa_enabled flag.
type Manager struct {
	pool *pgxpool.Pool
	totp *TOTPManager
}

// NewManager creates a new MFA Manager.
func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{
		pool: pool,
		totp: NewTOTPManager(),
	}
}

// EnableTOTP generates a new TOTP secret for the user and stores it
// (unverified) in the database. Returns the raw secret, otpauth:// URL,
// and a base64-encoded QR code image.
func (m *Manager) EnableTOTP(ctx context.Context, userID, issuer string) (string, string, string, error) {
	secret, qrURL, qrImage, err := m.totp.GenerateSecret(issuer, userID)
	if err != nil {
		return "", "", "", fmt.Errorf("generating TOTP secret: %w", err)
	}

	id := uuid.New()
	_, err = m.pool.Exec(ctx,
		`INSERT INTO mfa_totp (id, user_id, secret, verified, created_at)
		 VALUES ($1, $2, $3, false, $4)
		 ON CONFLICT (user_id) DO UPDATE
		 SET secret = EXCLUDED.secret, verified = false, created_at = EXCLUDED.created_at`,
		id, userID, []byte(secret), time.Now().UTC(),
	)
	if err != nil {
		return "", "", "", fmt.Errorf("storing TOTP secret: %w", err)
	}

	return secret, qrURL, qrImage, nil
}

// VerifyTOTP validates a TOTP code for the user. On the first successful
// validation it marks the TOTP entry as verified.
func (m *Manager) VerifyTOTP(ctx context.Context, userID, code string) (bool, error) {
	var secret []byte
	var verified bool
	err := m.pool.QueryRow(ctx,
		`SELECT secret, verified FROM mfa_totp WHERE user_id = $1`, userID,
	).Scan(&secret, &verified)
	if err != nil {
		return false, fmt.Errorf("fetching TOTP secret: %w", err)
	}

	if !m.totp.Validate(string(secret), code) {
		return false, nil
	}

	if !verified {
		_, err = m.pool.Exec(ctx,
			`UPDATE mfa_totp SET verified = true WHERE user_id = $1`, userID,
		)
		if err != nil {
			return false, fmt.Errorf("marking TOTP as verified: %w", err)
		}
	}

	return true, nil
}

// DisableTOTP removes the TOTP configuration for the user.
func (m *Manager) DisableTOTP(ctx context.Context, userID string) error {
	_, err := m.pool.Exec(ctx, `DELETE FROM mfa_totp WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("deleting TOTP: %w", err)
	}
	return nil
}

const (
	backupCodeCount  = 10
	backupCodeLength = 8
	backupCodeChars  = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// GenerateBackupCodes creates a fresh set of 10 single-use backup codes for
// the user, replacing any existing codes. Returns the plaintext codes (the
// only time they are available).
func (m *Manager) GenerateBackupCodes(ctx context.Context, userID string) ([]string, error) {
	codes := make([]string, backupCodeCount)
	for i := range codes {
		code, err := randomString(backupCodeLength, backupCodeChars)
		if err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		codes[i] = code
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Remove old codes.
	_, err = tx.Exec(ctx, `DELETE FROM mfa_backup_codes WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("deleting old backup codes: %w", err)
	}

	for _, code := range codes {
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hashing backup code: %w", err)
		}
		id := uuid.New()
		_, err = tx.Exec(ctx,
			`INSERT INTO mfa_backup_codes (id, user_id, code_hash, used) VALUES ($1, $2, $3, false)`,
			id, userID, string(hash),
		)
		if err != nil {
			return nil, fmt.Errorf("storing backup code: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing backup codes: %w", err)
	}

	return codes, nil
}

// ValidateBackupCode checks the code against unused backup codes for the user.
// If valid the matching code is marked as used (single-use).
func (m *Manager) ValidateBackupCode(ctx context.Context, userID, code string) (bool, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, code_hash FROM mfa_backup_codes WHERE user_id = $1 AND used = false`, userID,
	)
	if err != nil {
		return false, fmt.Errorf("fetching backup codes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return false, fmt.Errorf("scanning backup code: %w", err)
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(code)) == nil {
			_, err = m.pool.Exec(ctx,
				`UPDATE mfa_backup_codes SET used = true WHERE id = $1`, id,
			)
			if err != nil {
				return false, fmt.Errorf("marking backup code as used: %w", err)
			}
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating backup codes: %w", err)
	}

	return false, nil
}

// GetUserMFAStatus returns the current MFA status for a user.
func (m *Manager) GetUserMFAStatus(ctx context.Context, userID string) (*MFAStatus, error) {
	status := &MFAStatus{}

	// mfa_enabled flag on users table.
	err := m.pool.QueryRow(ctx,
		`SELECT mfa_enabled FROM users WHERE id = $1`, userID,
	).Scan(&status.MFAEnabled)
	if err != nil {
		return nil, fmt.Errorf("fetching mfa_enabled: %w", err)
	}

	// TOTP status.
	var totpCount int
	var verifiedCount int
	err = m.pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE verified) FROM mfa_totp WHERE user_id = $1`, userID,
	).Scan(&totpCount, &verifiedCount)
	if err != nil {
		return nil, fmt.Errorf("fetching TOTP status: %w", err)
	}
	status.TOTPConfigured = totpCount > 0
	status.TOTPVerified = verifiedCount > 0

	// WebAuthn credential count.
	err = m.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mfa_webauthn WHERE user_id = $1`, userID,
	).Scan(&status.WebAuthnCount)
	if err != nil {
		return nil, fmt.Errorf("fetching WebAuthn count: %w", err)
	}

	// Remaining backup codes.
	err = m.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mfa_backup_codes WHERE user_id = $1 AND used = false`, userID,
	).Scan(&status.BackupCodesLeft)
	if err != nil {
		return nil, fmt.Errorf("fetching backup codes count: %w", err)
	}

	return status, nil
}

// SetMFAEnabled updates the users.mfa_enabled flag.
func (m *Manager) SetMFAEnabled(ctx context.Context, userID string, enabled bool) error {
	_, err := m.pool.Exec(ctx,
		`UPDATE users SET mfa_enabled = $1 WHERE id = $2`, enabled, userID,
	)
	if err != nil {
		return fmt.Errorf("updating mfa_enabled: %w", err)
	}
	return nil
}

// randomString generates a cryptographically secure random string of the
// given length using characters from the provided alphabet.
func randomString(length int, alphabet string) (string, error) {
	max := big.NewInt(int64(len(alphabet)))
	buf := make([]byte, length)
	for i := range buf {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[idx.Int64()]
	}
	return string(buf), nil
}
