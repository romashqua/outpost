package mfa

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/pquerna/otp/totp"
)

// TOTPManager handles TOTP secret generation and code validation.
type TOTPManager struct{}

// NewTOTPManager creates a new TOTPManager.
func NewTOTPManager() *TOTPManager {
	return &TOTPManager{}
}

// GenerateSecret creates a new TOTP secret and returns the secret string,
// an otpauth:// URL, and a base64-encoded PNG QR code image.
func (m *TOTPManager) GenerateSecret(issuer, account string) (secret, qrURL, qrImage string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("generating TOTP secret: %w", err)
	}

	// Generate QR code PNG image.
	img, err := key.Image(256, 256)
	if err != nil {
		return "", "", "", fmt.Errorf("generating QR code image: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", "", "", fmt.Errorf("encoding QR code PNG: %w", err)
	}

	b64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	return key.Secret(), key.URL(), b64, nil
}

// Validate checks whether the given TOTP code is valid for the secret.
func (m *TOTPManager) Validate(secret, code string) bool {
	return totp.Validate(code, secret)
}
