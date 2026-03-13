package mfa

import (
	"fmt"

	"github.com/pquerna/otp/totp"
)

// TOTPManager handles TOTP secret generation and code validation.
type TOTPManager struct{}

// NewTOTPManager creates a new TOTPManager.
func NewTOTPManager() *TOTPManager {
	return &TOTPManager{}
}

// GenerateSecret creates a new TOTP secret and returns the secret string
// and an otpauth:// URL suitable for encoding as a QR code.
func (m *TOTPManager) GenerateSecret(issuer, account string) (secret string, qrURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating TOTP secret: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// Validate checks whether the given TOTP code is valid for the secret.
func (m *TOTPManager) Validate(secret, code string) bool {
	return totp.Validate(code, secret)
}
