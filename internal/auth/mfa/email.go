package mfa

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const emailTokenTTL = 5 * time.Minute

type emailToken struct {
	Code      string
	ExpiresAt time.Time
}

// EmailTokenManager handles short-lived 6-digit email verification codes.
// Tokens are stored in-memory for now; swap to Redis for multi-instance
// deployments later.
type EmailTokenManager struct {
	pool   *pgxpool.Pool
	tokens sync.Map // key: userID, value: emailToken
}

// NewEmailTokenManager creates a new EmailTokenManager.
func NewEmailTokenManager(pool *pgxpool.Pool) *EmailTokenManager {
	return &EmailTokenManager{pool: pool}
}

// SendToken generates a 6-digit code for the user and stores it with a 5
// minute TTL. The code is returned so the caller can deliver it (email
// sending will be wired in later). The email parameter is accepted for
// future use when the mailer is integrated.
func (m *EmailTokenManager) SendToken(_ string, email string) (string, error) {
	_ = email // will be used by mailer integration

	code, err := generateDigitCode(6)
	if err != nil {
		return "", fmt.Errorf("generating email token: %w", err)
	}

	m.tokens.Store(email, emailToken{
		Code:      code,
		ExpiresAt: time.Now().Add(emailTokenTTL),
	})

	return code, nil
}

// ValidateToken checks whether the provided code matches the stored token
// for the user. A valid token is consumed immediately (single-use).
func (m *EmailTokenManager) ValidateToken(userID, code string) bool {
	val, ok := m.tokens.Load(userID)
	if !ok {
		return false
	}

	tok := val.(emailToken)
	if time.Now().After(tok.ExpiresAt) {
		m.tokens.Delete(userID)
		return false
	}

	if tok.Code != code {
		return false
	}

	m.tokens.Delete(userID)
	return true
}

// generateDigitCode returns a cryptographically random numeric string of
// the given length.
func generateDigitCode(length int) (string, error) {
	max := big.NewInt(10)
	buf := make([]byte, length)
	for i := range buf {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = '0' + byte(n.Int64())
	}
	return string(buf), nil
}
