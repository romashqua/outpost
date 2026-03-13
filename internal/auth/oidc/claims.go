package oidc

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IDTokenClaims represents the standard OIDC ID Token claims.
type IDTokenClaims struct {
	// Standard OIDC claims.
	Nonce             string `json:"nonce,omitempty"`
	Email             string `json:"email,omitempty"`
	EmailVerified     bool   `json:"email_verified,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Groups            []string `json:"groups,omitempty"`
	jwt.RegisteredClaims
}

// UserInfo holds the user data needed to build OIDC claims.
type UserInfo struct {
	UserID        string
	Email         string
	EmailVerified bool
	Name          string
	Username      string
	Groups        []string
}

// BuildIDTokenClaims constructs IDTokenClaims from user data and request parameters.
func BuildIDTokenClaims(user UserInfo, issuer, clientID, nonce string, lifetime time.Duration) IDTokenClaims {
	now := time.Now()
	return IDTokenClaims{
		Nonce:             nonce,
		Email:             user.Email,
		EmailVerified:     user.EmailVerified,
		Name:              user.Name,
		PreferredUsername: user.Username,
		Groups:            user.Groups,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   user.UserID,
			Audience:  jwt.ClaimStrings{clientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(lifetime)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
}

// AccessTokenClaims represents claims embedded in an OIDC access token.
type AccessTokenClaims struct {
	Scope    string `json:"scope,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	jwt.RegisteredClaims
}
