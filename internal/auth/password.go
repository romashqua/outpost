package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a bcrypt hash with a plaintext password.
// Returns nil on success, or an error if they don't match.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
