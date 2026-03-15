package auth

import (
	"strings"
	"unicode"
)

// ValidatePasswordPolicy checks that the password meets the security policy.
// It returns nil if all requirements are satisfied, or an error whose message
// lists every unmet requirement (one per line) so the caller can relay it to
// the user as-is.
func ValidatePasswordPolicy(password string) error {
	var missing []string

	if len(password) < 8 {
		missing = append(missing, "at least 8 characters")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	if !hasUpper {
		missing = append(missing, "at least one uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "at least one lowercase letter")
	}
	if !hasDigit {
		missing = append(missing, "at least one digit")
	}
	if !hasSpecial {
		missing = append(missing, "at least one special character")
	}

	if len(missing) > 0 {
		return &PasswordPolicyError{Requirements: missing}
	}
	return nil
}

// PasswordPolicyError contains the list of unmet password requirements.
type PasswordPolicyError struct {
	Requirements []string
}

func (e *PasswordPolicyError) Error() string {
	return "password does not meet requirements: " + strings.Join(e.Requirements, "; ")
}
