package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rsaKeyBits = 2048

// Provider is the built-in OIDC identity provider ("Log in with Outpost").
// It implements the Authorization Code flow with PKCE, token issuance,
// UserInfo, JWKS, and OpenID Connect Discovery.
type Provider struct {
	store      *Store
	issuer     string
	signingKey *rsa.PrivateKey
	keyID      string
}

// NewProvider creates a new OIDC provider. If signingKey is nil an RSA key
// pair is generated automatically (suitable for development / single-node).
func NewProvider(pool *pgxpool.Pool, issuer string, signingKey *rsa.PrivateKey) *Provider {
	if signingKey == nil {
		var err error
		signingKey, err = rsa.GenerateKey(rand.Reader, rsaKeyBits)
		if err != nil {
			panic(fmt.Sprintf("oidc: failed to generate RSA key: %v", err))
		}
		slog.Info("oidc: generated ephemeral RSA signing key", "bits", rsaKeyBits)
	}

	// Derive a stable key ID from the public key modulus so relying parties
	// can cache JWKS entries across restarts when using a persistent key.
	kid := deriveKeyID(signingKey)

	return &Provider{
		store:      NewStore(pool),
		issuer:     issuer,
		signingKey: signingKey,
		keyID:      kid,
	}
}

// Routes returns a chi.Router with all OIDC endpoints mounted.
//
//	GET  /.well-known/openid-configuration
//	GET  /authorize
//	POST /token
//	GET  /userinfo
//	GET  /jwks
func (p *Provider) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/.well-known/openid-configuration", p.discovery)
	r.Get("/authorize", p.authorize)
	r.Post("/token", p.token)
	r.Get("/userinfo", p.userinfo)
	r.Get("/jwks", p.jwks)

	return r
}

// deriveKeyID produces a short, deterministic key ID from the RSA public key.
func deriveKeyID(key *rsa.PrivateKey) string {
	h := sha256.Sum256(key.PublicKey.N.Bytes())
	return hex.EncodeToString(h[:8])
}

// generateRandomCode produces a URL-safe random string of the given byte length.
func generateRandomCode(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("oidc: generating random code: %w", err)
	}
	return hex.EncodeToString(b), nil
}
