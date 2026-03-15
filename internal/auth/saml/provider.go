// Package saml implements a SAML 2.0 Service Provider for Outpost VPN.
//
// This package provides the configuration, data types, route handlers, and
// metadata generation for SAML 2.0 SSO. It uses the crewjam/saml library
// for SAML XML parsing, AuthnRequest building, and assertion validation.
package saml

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/romashqua/outpost/internal/auth"
)

// Config holds the SAML 2.0 Service Provider configuration.
type Config struct {
	EntityID          string       // SP entity ID, e.g., https://vpn.example.com/saml
	ACSURL            string       // Assertion Consumer Service URL
	MetadataURL       string       // URL where SP metadata is served
	IDPMetadataURL    string       // Identity Provider metadata URL (fetched at startup)
	IDPMetadata       string       // Raw IDP metadata XML (alternative to URL)
	CertFile          string       // Path to SP certificate (PEM)
	KeyFile           string       // Path to SP private key (PEM)
	SignRequests      bool         // Whether to sign AuthnRequests
	AllowIDPInitiated bool         // Whether to accept IDP-initiated SSO
	AttributeMap      AttributeMap // Maps SAML attributes to user fields
	JWTSecret         string       // JWT secret for generating session tokens
	BaseURL           string       // Base URL for redirects after auth (e.g., https://vpn.example.com)
}

// AttributeMap maps SAML assertion attributes to user database fields.
type AttributeMap struct {
	Email     string // default: "email" or "urn:oid:0.9.2342.19200300.100.1.3"
	FirstName string // default: "givenName"
	LastName  string // default: "sn"
	Username  string // default: "uid"
	Groups    string // default: "memberOf"
}

// DefaultAttributeMap returns the default SAML attribute mapping.
func DefaultAttributeMap() AttributeMap {
	return AttributeMap{
		Email:     "urn:oid:0.9.2342.19200300.100.1.3",
		FirstName: "givenName",
		LastName:  "sn",
		Username:  "uid",
		Groups:    "memberOf",
	}
}

// SAMLUser represents a user extracted from a SAML assertion.
type SAMLUser struct {
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Username  string   `json:"username"`
	Groups    []string `json:"groups,omitempty"`
	NameID    string   `json:"name_id"`
}

// ServiceProvider implements a SAML 2.0 SP with HTTP route handlers.
type ServiceProvider struct {
	cfg    Config
	pool   *pgxpool.Pool
	logger *slog.Logger
	sp     *saml.ServiceProvider // crewjam/saml SP instance
}

// NewServiceProvider creates a new SAML Service Provider.
func NewServiceProvider(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) *ServiceProvider {
	if cfg.AttributeMap == (AttributeMap{}) {
		cfg.AttributeMap = DefaultAttributeMap()
	}
	// Derive BaseURL from ACSURL if not explicitly set.
	if cfg.BaseURL == "" && cfg.ACSURL != "" {
		if u, err := url.Parse(cfg.ACSURL); err == nil {
			cfg.BaseURL = u.Scheme + "://" + u.Host
		}
	}

	s := &ServiceProvider{
		cfg:    cfg,
		pool:   pool,
		logger: logger,
	}

	// Initialize the crewjam/saml ServiceProvider if cert/key are configured.
	if err := s.initSAMLSP(); err != nil {
		logger.Error("failed to initialize SAML service provider", "error", err)
	}

	return s
}

// initSAMLSP initializes the underlying crewjam/saml.ServiceProvider using
// the configured certificate, private key, and IDP metadata.
func (sp *ServiceProvider) initSAMLSP() error {
	if sp.cfg.CertFile == "" || sp.cfg.KeyFile == "" {
		return fmt.Errorf("saml: certificate and key file paths are required")
	}

	// Load SP key pair.
	keyPair, err := tls.LoadX509KeyPair(sp.cfg.CertFile, sp.cfg.KeyFile)
	if err != nil {
		return fmt.Errorf("saml: loading key pair: %w", err)
	}
	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return fmt.Errorf("saml: parsing certificate: %w", err)
	}

	// Parse the ACS URL to derive the root URL.
	acsURL, err := url.Parse(sp.cfg.ACSURL)
	if err != nil {
		return fmt.Errorf("saml: parsing ACS URL: %w", err)
	}

	// Build the metadata URL from the entity ID if not explicitly set.
	metadataURL, err := url.Parse(sp.cfg.EntityID + "/metadata")
	if err != nil {
		metadataURL = acsURL
	}

	// Fetch or parse IDP metadata.
	idpMetadata, err := sp.loadIDPMetadata()
	if err != nil {
		return fmt.Errorf("saml: loading IDP metadata: %w", err)
	}

	sp.sp = &saml.ServiceProvider{
		EntityID:          sp.cfg.EntityID,
		Key:               keyPair.PrivateKey.(crypto.Signer),
		Certificate:       keyPair.Leaf,
		MetadataURL:       *metadataURL,
		AcsURL:            *acsURL,
		IDPMetadata:       idpMetadata,
		AllowIDPInitiated: sp.cfg.AllowIDPInitiated,
	}

	return nil
}

// loadIDPMetadata fetches IDP metadata from URL or parses the raw XML string.
func (sp *ServiceProvider) loadIDPMetadata() (*saml.EntityDescriptor, error) {
	if sp.cfg.IDPMetadataURL != "" {
		idpURL, err := url.Parse(sp.cfg.IDPMetadataURL)
		if err != nil {
			return nil, fmt.Errorf("parsing IDP metadata URL: %w", err)
		}
		metadata, err := samlsp.FetchMetadata(context.Background(), http.DefaultClient, *idpURL)
		if err != nil {
			return nil, fmt.Errorf("fetching IDP metadata from %s: %w", sp.cfg.IDPMetadataURL, err)
		}
		return metadata, nil
	}

	if sp.cfg.IDPMetadata != "" {
		var metadata saml.EntityDescriptor
		if err := xml.Unmarshal([]byte(sp.cfg.IDPMetadata), &metadata); err != nil {
			return nil, fmt.Errorf("parsing IDP metadata XML: %w", err)
		}
		return &metadata, nil
	}

	return nil, fmt.Errorf("either IDPMetadataURL or IDPMetadata must be provided")
}

// Routes returns a chi.Router with the SAML SP endpoints mounted.
//
//	GET  /metadata — SP metadata XML
//	POST /acs      — Assertion Consumer Service
//	GET  /login    — Initiate SAML login (redirect to IDP)
//	GET  /logout   — SAML Single Logout
func (sp *ServiceProvider) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/metadata", sp.handleMetadata)
	r.Post("/acs", sp.handleACS)
	r.Get("/login", sp.handleLogin)
	r.Get("/logout", sp.handleLogout)
	return r
}

// handleMetadata serves the SP metadata XML document using the crewjam/saml library.
func (sp *ServiceProvider) handleMetadata(w http.ResponseWriter, r *http.Request) {
	_ = r
	if sp.sp != nil {
		// Use crewjam/saml to generate metadata (includes signing key info).
		buf, _ := xml.MarshalIndent(sp.sp.Metadata(), "", "  ")
		w.Header().Set("Content-Type", "application/samlmetadata+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(xml.Header))
		_, _ = w.Write(buf)
		return
	}

	// Fallback to manual metadata generation if SP not initialized.
	data, err := sp.GenerateMetadata()
	if err != nil {
		sp.logger.Error("failed to generate SAML metadata", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleLogin initiates the SAML authentication flow by redirecting to the IDP.
func (sp *ServiceProvider) handleLogin(w http.ResponseWriter, r *http.Request) {
	if sp.sp == nil {
		sp.logger.Error("SAML service provider not initialized")
		http.Error(w, "SAML not configured", http.StatusInternalServerError)
		return
	}

	sp.logger.Info("SAML login initiated", "remote_addr", r.RemoteAddr)

	// Build the AuthnRequest using crewjam/saml.
	authReq, err := sp.sp.MakeAuthenticationRequest(
		sp.sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		sp.logger.Error("failed to create SAML AuthnRequest", "error", err)
		http.Error(w, "failed to initiate SAML login", http.StatusInternalServerError)
		return
	}

	// Build the redirect URL with the deflated, base64-encoded AuthnRequest.
	redirectURL, err := authReq.Redirect("", sp.sp)
	if err != nil {
		sp.logger.Error("failed to build SAML redirect URL", "error", err)
		http.Error(w, "failed to initiate SAML login", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// handleACS processes the SAML Response posted by the IDP.
func (sp *ServiceProvider) handleACS(w http.ResponseWriter, r *http.Request) {
	if sp.sp == nil {
		sp.logger.Error("SAML service provider not initialized")
		http.Error(w, "SAML not configured", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		sp.logger.Error("failed to parse ACS form", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if r.FormValue("SAMLResponse") == "" {
		http.Error(w, "missing SAMLResponse", http.StatusBadRequest)
		return
	}

	user, err := sp.ProcessAssertion(r.Context(), r)
	if err != nil {
		sp.logger.Error("failed to process SAML assertion", "error", err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	sp.logger.Info("SAML authentication successful",
		"username", user.Username,
		"email", user.Email,
		"name_id", user.NameID,
	)

	// Create or update user in the database.
	userID, isAdmin, err := sp.upsertUser(r.Context(), user)
	if err != nil {
		sp.logger.Error("failed to upsert SAML user", "error", err)
		http.Error(w, "failed to create user account", http.StatusInternalServerError)
		return
	}

	// Generate a JWT session token.
	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := auth.GenerateToken(sp.cfg.JWTSecret, auth.TokenClaims{
		UserID:   userID,
		Username: user.Username,
		Email:    user.Email,
		IsAdmin:  isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	if err != nil {
		sp.logger.Error("failed to generate JWT for SAML user", "error", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	// Redirect to the frontend with the token as a query parameter.
	// The frontend reads this token and stores it in local storage.
	redirectURL := sp.cfg.BaseURL + "/login?token=" + url.QueryEscape(token) +
		"&expires_at=" + fmt.Sprintf("%d", expiresAt.Unix())
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleLogout handles SAML Single Logout (SLO).
func (sp *ServiceProvider) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sp.sp == nil {
		sp.logger.Error("SAML service provider not initialized")
		http.Error(w, "SAML not configured", http.StatusInternalServerError)
		return
	}

	sp.logger.Info("SAML logout initiated", "remote_addr", r.RemoteAddr)

	// Extract the NameID from the JWT token in the Authorization header or cookie
	// to build the LogoutRequest for the IDP.
	nameID := ""
	tokenStr := r.Header.Get("Authorization")
	if len(tokenStr) > 7 && strings.HasPrefix(tokenStr, "Bearer ") {
		tokenStr = tokenStr[7:]
		if claims, err := auth.ValidateToken(sp.cfg.JWTSecret, tokenStr); err == nil {
			// Use the email as the NameID (matching what the IDP typically sends).
			nameID = claims.Email
		}
	}

	// Also accept name_id as a query parameter.
	if nameID == "" {
		nameID = r.URL.Query().Get("name_id")
	}

	if nameID == "" {
		// Without a NameID, we cannot build a proper LogoutRequest.
		// Redirect to the application root as a simple logout.
		sp.logger.Warn("SAML logout without NameID, performing local-only logout")
		http.Redirect(w, r, sp.cfg.BaseURL+"/login", http.StatusFound)
		return
	}

	// Find the IDP's SLO endpoint.
	sloURL := sp.getIDPSLOLocation()
	if sloURL == "" {
		// IDP does not support SLO; perform local-only logout.
		sp.logger.Warn("IDP does not have SLO endpoint configured, performing local-only logout")
		http.Redirect(w, r, sp.cfg.BaseURL+"/login", http.StatusFound)
		return
	}

	// Build the SAML LogoutRequest.
	logoutReq, err := sp.sp.MakeLogoutRequest(sloURL, nameID)
	if err != nil {
		sp.logger.Error("failed to create SAML LogoutRequest", "error", err)
		http.Redirect(w, r, sp.cfg.BaseURL+"/login", http.StatusFound)
		return
	}

	// Redirect to the IDP's SLO endpoint with the LogoutRequest.
	redirectURL := logoutReq.Redirect("")
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// getIDPSLOLocation extracts the IDP's Single Logout Service URL from metadata.
func (sp *ServiceProvider) getIDPSLOLocation() string {
	if sp.sp.IDPMetadata == nil {
		return ""
	}
	for _, desc := range sp.sp.IDPMetadata.IDPSSODescriptors {
		for _, slo := range desc.SingleLogoutServices {
			if slo.Binding == saml.HTTPRedirectBinding {
				return slo.Location
			}
		}
		// Fall back to POST binding if redirect is not available.
		for _, slo := range desc.SingleLogoutServices {
			if slo.Binding == saml.HTTPPostBinding {
				return slo.Location
			}
		}
	}
	return ""
}

// ProcessAssertion parses and validates a SAML Response from the given HTTP request,
// then extracts user attributes according to the configured attribute map.
// The request must contain a parsed form with the SAMLResponse field.
func (sp *ServiceProvider) ProcessAssertion(ctx context.Context, r *http.Request) (*SAMLUser, error) {
	_ = ctx

	if sp.sp == nil {
		return nil, fmt.Errorf("saml: service provider not initialized")
	}

	// Parse and validate the SAML response. The crewjam/saml library handles:
	// - Base64 decoding
	// - XML parsing
	// - Signature validation against IDP certificate
	// - Condition checks (NotBefore, NotOnOrAfter, Audience)
	possibleIDs := []string{saml.HTTPPostBinding}
	assertion, err := sp.sp.ParseResponse(r, possibleIDs)
	if err != nil {
		return nil, fmt.Errorf("saml: validating response: %w", err)
	}

	// Extract user attributes from the assertion.
	user := &SAMLUser{
		NameID: assertion.Subject.NameID.Value,
	}

	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			values := attrValues(attr)
			if len(values) == 0 {
				continue
			}
			switch {
			case matchesAttr(attr.Name, attr.FriendlyName, sp.cfg.AttributeMap.Email, "email"):
				user.Email = values[0]
			case matchesAttr(attr.Name, attr.FriendlyName, sp.cfg.AttributeMap.FirstName, "givenName"):
				user.FirstName = values[0]
			case matchesAttr(attr.Name, attr.FriendlyName, sp.cfg.AttributeMap.LastName, "sn"):
				user.LastName = values[0]
			case matchesAttr(attr.Name, attr.FriendlyName, sp.cfg.AttributeMap.Username, "uid"):
				user.Username = values[0]
			case matchesAttr(attr.Name, attr.FriendlyName, sp.cfg.AttributeMap.Groups, "memberOf"):
				user.Groups = values
			}
		}
	}

	// Fall back to NameID for email/username if attributes are missing.
	if user.Email == "" {
		user.Email = user.NameID
	}
	if user.Username == "" {
		// Derive username from email (part before @).
		if idx := strings.Index(user.Email, "@"); idx > 0 {
			user.Username = user.Email[:idx]
		} else {
			user.Username = user.Email
		}
	}

	if user.Email == "" {
		return nil, fmt.Errorf("saml: no email found in assertion")
	}

	return user, nil
}

// attrValues extracts string values from a SAML attribute.
func attrValues(attr saml.Attribute) []string {
	var vals []string
	for _, v := range attr.Values {
		if v.Value != "" {
			vals = append(vals, v.Value)
		}
	}
	return vals
}

// matchesAttr checks whether a SAML attribute name or friendly name matches
// the configured mapping or a fallback name.
func matchesAttr(name, friendlyName, configured, fallback string) bool {
	if configured != "" {
		return name == configured || friendlyName == configured
	}
	return name == fallback || friendlyName == fallback
}

// upsertUser creates or updates a user in the database based on SAML assertion data.
// Returns the user ID and admin status.
func (sp *ServiceProvider) upsertUser(ctx context.Context, user *SAMLUser) (string, bool, error) {
	var userID string
	var isAdmin bool

	// Try to find existing user by email.
	err := sp.pool.QueryRow(ctx,
		`SELECT id, is_admin FROM users WHERE email = $1`, user.Email,
	).Scan(&userID, &isAdmin)

	if err != nil {
		// User does not exist; create a new one.
		// SAML users have no password (they authenticate via IDP).
		err = sp.pool.QueryRow(ctx,
			`INSERT INTO users (username, email, first_name, last_name, is_active, enrolled_at)
			 VALUES ($1, $2, $3, $4, true, now())
			 ON CONFLICT (email) DO UPDATE
			 SET first_name = EXCLUDED.first_name,
			     last_name = EXCLUDED.last_name,
			     updated_at = now()
			 RETURNING id, is_admin`,
			user.Username, user.Email, user.FirstName, user.LastName,
		).Scan(&userID, &isAdmin)
		if err != nil {
			return "", false, fmt.Errorf("inserting SAML user: %w", err)
		}

		sp.logger.Info("created new SAML user", "user_id", userID, "email", user.Email)
	} else {
		// Update existing user with fresh attributes from IDP.
		_, err = sp.pool.Exec(ctx,
			`UPDATE users SET first_name = $1, last_name = $2, updated_at = now() WHERE id = $3`,
			user.FirstName, user.LastName, userID,
		)
		if err != nil {
			sp.logger.Warn("failed to update SAML user attributes", "user_id", userID, "error", err)
			// Non-fatal: proceed with existing data.
		}
	}

	return userID, isAdmin, nil
}

// spMetadata represents the SP metadata XML document (SAML 2.0).
type spMetadata struct {
	XMLName         xml.Name        `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
	EntityID        string          `xml:"entityID,attr"`
	SPSSODescriptor spSSODescriptor `xml:"SPSSODescriptor"`
}

type spSSODescriptor struct {
	XMLName                    xml.Name                   `xml:"urn:oasis:names:tc:SAML:2.0:metadata SPSSODescriptor"`
	ProtocolSupportEnumeration string                     `xml:"protocolSupportEnumeration,attr"`
	AuthnRequestsSigned        bool                       `xml:"AuthnRequestsSigned,attr"`
	WantAssertionsSigned       bool                       `xml:"WantAssertionsSigned,attr"`
	NameIDFormats              []nameIDFormat              `xml:"NameIDFormat"`
	AssertionConsumerServices  []assertionConsumerService  `xml:"AssertionConsumerService"`
}

type nameIDFormat struct {
	XMLName xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata NameIDFormat"`
	Value   string   `xml:",chardata"`
}

type assertionConsumerService struct {
	XMLName  xml.Name `xml:"urn:oasis:names:tc:SAML:2.0:metadata AssertionConsumerService"`
	Binding  string   `xml:"Binding,attr"`
	Location string   `xml:"Location,attr"`
	Index    int      `xml:"index,attr"`
}

// GenerateMetadata produces the SP metadata XML document (fallback when
// the crewjam/saml SP is not initialized).
func (sp *ServiceProvider) GenerateMetadata() ([]byte, error) {
	md := spMetadata{
		EntityID: sp.cfg.EntityID,
		SPSSODescriptor: spSSODescriptor{
			ProtocolSupportEnumeration: "urn:oasis:names:tc:SAML:2.0:protocol",
			AuthnRequestsSigned:        sp.cfg.SignRequests,
			WantAssertionsSigned:       true,
			NameIDFormats: []nameIDFormat{
				{Value: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"},
				{Value: "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"},
				{Value: "urn:oasis:names:tc:SAML:2.0:nameid-format:transient"},
			},
			AssertionConsumerServices: []assertionConsumerService{
				{
					Binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
					Location: sp.cfg.ACSURL,
					Index:    0,
				},
			},
		},
	}

	output, err := xml.MarshalIndent(md, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("saml: marshalling metadata: %w", err)
	}

	// Prepend XML declaration.
	header := []byte(xml.Header)
	result := make([]byte, 0, len(header)+len(output))
	result = append(result, header...)
	result = append(result, output...)
	return result, nil
}

// ValidUntil returns a metadata validity timestamp (24 hours from now).
// Used when generating metadata with a validUntil attribute.
func ValidUntil() time.Time {
	return time.Now().UTC().Add(24 * time.Hour)
}

