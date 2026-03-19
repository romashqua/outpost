package oidc

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/romashqua/outpost/internal/auth"
)

const (
	authCodeLifetime   = 10 * time.Minute
	accessTokenLifetime = 1 * time.Hour
	idTokenLifetime    = 1 * time.Hour
)

// @Summary OpenID Connect Discovery
// @Description Return the OpenID Connect Discovery document (RFC 8414).
// @Tags OIDC
// @Produce json
// @Success 200 {object} object
// @Router /oidc/.well-known/openid-configuration [get]
//
// discovery handles GET /.well-known/openid-configuration.
func (p *Provider) discovery(w http.ResponseWriter, r *http.Request) {
	doc := map[string]any{
		"issuer":                                p.issuer,
		"authorization_endpoint":                p.issuer + "/authorize",
		"token_endpoint":                        p.issuer + "/token",
		"userinfo_endpoint":                     p.issuer + "/userinfo",
		"jwks_uri":                              p.issuer + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "groups"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "nonce",
			"email", "email_verified", "name", "preferred_username", "groups",
		},
		"code_challenge_methods_supported": []string{"S256"},
		"grant_types_supported":            []string{"authorization_code"},
	}
	writeJSON(w, http.StatusOK, doc)
}

// @Summary OIDC Authorization
// @Description Initiate the OAuth 2.0 Authorization Code flow with optional PKCE (S256).
// @Tags OIDC
// @Param client_id query string true "OAuth2 client ID"
// @Param redirect_uri query string true "Redirect URI registered with the client"
// @Param response_type query string true "Must be 'code'"
// @Param scope query string false "Space-separated scopes (openid, profile, email, groups)"
// @Param state query string false "Opaque state value for CSRF protection"
// @Param nonce query string false "Nonce for ID token replay protection"
// @Param code_challenge query string false "PKCE code challenge (S256)"
// @Param code_challenge_method query string false "Must be 'S256' if provided"
// @Success 302 "Redirect to redirect_uri with authorization code"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Security BearerAuth
// @Router /oidc/authorize [get]
//
// authorize handles GET /authorize — the authorization endpoint.
// In a real deployment a login UI would be shown; here we expect the user_id
// to already be established via session (passed as a query parameter for now).
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	scope := q.Get("scope")
	state := q.Get("state")
	nonce := q.Get("nonce")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	// The user_id comes from the authenticated session. In production this
	// would be extracted from a session cookie; for the API layer we accept
	// it from context (set by auth middleware).
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		oidcError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	userID := claims.UserID

	// Validate client and redirect URI BEFORE using redirectURI in any response.
	client, err := p.store.GetClient(r.Context(), clientID)
	if err != nil {
		oidcError(w, http.StatusBadRequest, "invalid client_id")
		return
	}

	if !ValidateRedirectURI(client, redirectURI) {
		oidcError(w, http.StatusBadRequest, "invalid redirect_uri")
		return
	}

	if responseType != "code" {
		errorRedirect(w, r, redirectURI, state, "unsupported_response_type", "only response_type=code is supported")
		return
	}

	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		errorRedirect(w, r, redirectURI, state, "invalid_request", "only S256 code_challenge_method is supported")
		return
	}

	// Parse requested scopes, default to client-registered scopes.
	scopes := client.Scopes
	if scope != "" {
		scopes = strings.Split(scope, " ")
	}

	code, err := generateRandomCode(32)
	if err != nil {
		oidcError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now()
	ac := AuthCode{
		Code:                code,
		ClientID:            clientID,
		UserID:              userID,
		Scopes:              scopes,
		Nonce:               nonce,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           now.Add(authCodeLifetime),
		CreatedAt:           now,
	}

	if err := p.store.CreateAuthCode(r.Context(), ac); err != nil {
		oidcError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Redirect back with authorization code.
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		oidcError(w, http.StatusBadRequest, "malformed redirect_uri")
		return
	}
	params := redirectURL.Query()
	params.Set("code", code)
	if state != "" {
		params.Set("state", state)
	}
	redirectURL.RawQuery = params.Encode()
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// @Summary OIDC Token
// @Description Exchange an authorization code for access_token and id_token (RFC 6749 section 4.1.3).
// @Tags OIDC
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "Must be 'authorization_code'"
// @Param code formData string true "Authorization code"
// @Param redirect_uri formData string true "Must match the original authorize request"
// @Param client_id formData string true "OAuth2 client ID"
// @Param client_secret formData string false "Client secret (required for confidential clients)"
// @Param code_verifier formData string false "PKCE code verifier"
// @Success 200 {object} object "Token response with access_token, id_token, token_type, expires_in, scope"
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /oidc/token [post]
//
// token handles POST /token — the token endpoint.
func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, "invalid_request", "malformed form body", http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		tokenError(w, "unsupported_grant_type", "only authorization_code is supported", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	// Consume the authorization code (single-use).
	ac, err := p.store.ConsumeAuthCode(r.Context(), code)
	if err != nil {
		tokenError(w, "invalid_grant", "invalid or expired authorization code", http.StatusBadRequest)
		return
	}

	// Validate expiry.
	if time.Now().After(ac.ExpiresAt) {
		tokenError(w, "invalid_grant", "authorization code expired", http.StatusBadRequest)
		return
	}

	// client_id is required per RFC 6749 section 4.1.3.
	if clientID == "" {
		tokenError(w, "invalid_request", "client_id is required", http.StatusBadRequest)
		return
	}
	if clientID != ac.ClientID {
		tokenError(w, "invalid_grant", "client_id mismatch", http.StatusBadRequest)
		return
	}

	// Validate redirect_uri matches.
	if redirectURI != ac.RedirectURI {
		tokenError(w, "invalid_grant", "redirect_uri mismatch", http.StatusBadRequest)
		return
	}

	// Authenticate confidential clients — require secret if client has one.
	client, err := p.store.GetClient(r.Context(), ac.ClientID)
	if err != nil {
		tokenError(w, "invalid_client", "unknown client", http.StatusUnauthorized)
		return
	}
	if client.SecretHash != "" {
		if clientSecret == "" {
			tokenError(w, "invalid_client", "client_secret is required for confidential clients", http.StatusUnauthorized)
			return
		}
		if err := auth.CheckPassword(client.SecretHash, clientSecret); err != nil {
			tokenError(w, "invalid_client", "invalid client credentials", http.StatusUnauthorized)
			return
		}
	}

	// Validate PKCE.
	if ac.CodeChallenge != "" {
		if codeVerifier == "" {
			tokenError(w, "invalid_grant", "code_verifier required", http.StatusBadRequest)
			return
		}
		if !verifyPKCE(ac.CodeChallenge, ac.CodeChallengeMethod, codeVerifier) {
			tokenError(w, "invalid_grant", "PKCE verification failed", http.StatusBadRequest)
			return
		}
	}

	// Look up user information for building tokens.
	userInfo, err := p.store.GetUserInfo(r.Context(), ac.UserID)
	if err != nil {
		tokenError(w, "server_error", "failed to retrieve user", http.StatusInternalServerError)
		return
	}

	groups, _ := p.store.GetUserGroups(r.Context(), ac.UserID)
	userInfo.Groups = groups

	// Build ID token.
	idClaims := BuildIDTokenClaims(*userInfo, p.issuer, ac.ClientID, ac.Nonce, idTokenLifetime)
	idToken := jwt.NewWithClaims(jwt.SigningMethodRS256, idClaims)
	idToken.Header["kid"] = p.keyID
	idTokenStr, err := idToken.SignedString(p.signingKey)
	if err != nil {
		tokenError(w, "server_error", "failed to sign id_token", http.StatusInternalServerError)
		return
	}

	// Build access token.
	now := time.Now()
	accessClaims := AccessTokenClaims{
		Scope:    strings.Join(ac.Scopes, " "),
		ClientID: ac.ClientID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    p.issuer,
			Subject:   ac.UserID,
			Audience:  jwt.ClaimStrings{ac.ClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(accessTokenLifetime)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessToken.Header["kid"] = p.keyID
	accessTokenStr, err := accessToken.SignedString(p.signingKey)
	if err != nil {
		tokenError(w, "server_error", "failed to sign access_token", http.StatusInternalServerError)
		return
	}

	resp := map[string]any{
		"access_token": accessTokenStr,
		"token_type":   "Bearer",
		"expires_in":   int(accessTokenLifetime.Seconds()),
		"id_token":     idTokenStr,
		"scope":        strings.Join(ac.Scopes, " "),
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, resp)
}

// @Summary OIDC UserInfo
// @Description Return claims about the authenticated user (OpenID Connect Core section 5.3).
// @Tags OIDC
// @Produce json
// @Success 200 {object} object "UserInfo claims (sub, email, name, preferred_username, groups)"
// @Failure 401 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /oidc/userinfo [get]
//
// userinfo handles GET /userinfo — returns claims for the authenticated user.
func (p *Provider) userinfo(w http.ResponseWriter, r *http.Request) {
	tokenStr := extractBearerToken(r)
	if tokenStr == "" {
		oidcError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}

	// Parse the access token using our RSA public key.
	token, err := jwt.ParseWithClaims(tokenStr, &AccessTokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return &p.signingKey.PublicKey, nil
	})
	if err != nil {
		oidcError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		oidcError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	userInfo, err := p.store.GetUserInfo(r.Context(), claims.Subject)
	if err != nil {
		oidcError(w, http.StatusNotFound, "user not found")
		return
	}

	groups, _ := p.store.GetUserGroups(r.Context(), claims.Subject)

	resp := map[string]any{
		"sub":                claims.Subject,
		"email":              userInfo.Email,
		"email_verified":     userInfo.EmailVerified,
		"name":               userInfo.Name,
		"preferred_username": userInfo.Username,
	}
	if len(groups) > 0 {
		resp["groups"] = groups
	}
	writeJSON(w, http.StatusOK, resp)
}

// @Summary OIDC JWKS
// @Description Return the JSON Web Key Set containing the provider's RSA public signing key (RFC 7517).
// @Tags OIDC
// @Produce json
// @Success 200 {object} object "JWKS document with keys array"
// @Router /oidc/jwks [get]
//
// jwks handles GET /jwks — returns the JSON Web Key Set containing the public key.
func (p *Provider) jwks(w http.ResponseWriter, r *http.Request) {
	pub := &p.signingKey.PublicKey

	jwksDoc := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": p.keyID,
				"n":   base64URLEncode(pub.N.Bytes()),
				"e":   base64URLEncode(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSON(w, http.StatusOK, jwksDoc)
}

// verifyPKCE validates the code_verifier against the stored code_challenge using S256.
func verifyPKCE(challenge, method, verifier string) bool {
	if method == "" || method == "S256" {
		h := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(h[:])
		return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
	}
	return false
}

// base64URLEncode encodes bytes to unpadded base64url.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// extractBearerToken pulls the token from the Authorization: Bearer header.
func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found {
		return ""
	}
	return token
}

// writeJSON serializes v as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// tokenError writes a standard OAuth2 error response.
func tokenError(w http.ResponseWriter, errCode, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": description,
	})
}

// oidcError writes a JSON error response matching the API error contract.
func oidcError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message, "message": message})
}

// errorRedirect sends an OAuth2 error via redirect.
func errorRedirect(w http.ResponseWriter, r *http.Request, redirectURI, state, errCode, description string) {
	if redirectURI == "" {
		oidcError(w, http.StatusBadRequest, description)
		return
	}
	u, err := url.Parse(redirectURI)
	if err != nil {
		oidcError(w, http.StatusBadRequest, description)
		return
	}
	params := u.Query()
	params.Set("error", errCode)
	params.Set("error_description", description)
	if state != "" {
		params.Set("state", state)
	}
	u.RawQuery = params.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}
