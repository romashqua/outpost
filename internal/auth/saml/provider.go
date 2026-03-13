// Package saml implements a SAML 2.0 Service Provider for Outpost VPN.
//
// This package provides the configuration, data types, route handlers, and
// metadata generation for SAML 2.0 SSO. The actual SAML XML parsing and
// assertion validation should be implemented using "github.com/crewjam/saml"
// — the handler stubs and ProcessAssertion method contain TODO markers where
// that library should be integrated.
package saml

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
}

// NewServiceProvider creates a new SAML Service Provider.
func NewServiceProvider(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) *ServiceProvider {
	if cfg.AttributeMap == (AttributeMap{}) {
		cfg.AttributeMap = DefaultAttributeMap()
	}
	return &ServiceProvider{
		cfg:    cfg,
		pool:   pool,
		logger: logger,
	}
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

// handleMetadata serves the SP metadata XML document.
func (sp *ServiceProvider) handleMetadata(w http.ResponseWriter, r *http.Request) {
	_ = r
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

// handleACS processes the SAML Response posted by the IDP.
func (sp *ServiceProvider) handleACS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		sp.logger.Error("failed to parse ACS form", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	samlResponse := r.FormValue("SAMLResponse")
	if samlResponse == "" {
		http.Error(w, "missing SAMLResponse", http.StatusBadRequest)
		return
	}

	user, err := sp.ProcessAssertion(r.Context(), samlResponse)
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

	// TODO: Create or update user in database, generate session/JWT token,
	// and redirect to the application. For now, return the user as JSON.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(user)
}

// handleLogin initiates the SAML authentication flow by redirecting to the IDP.
func (sp *ServiceProvider) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r
	// TODO: Build a SAMLAuthnRequest, optionally sign it, and redirect
	// the user to the IDP's SSO URL with the request. Use crewjam/saml
	// to construct the AuthnRequest XML.
	//
	// Example flow:
	//   1. Build AuthnRequest with sp.cfg.EntityID, sp.cfg.ACSURL
	//   2. If sp.cfg.SignRequests, sign with SP private key
	//   3. Base64-encode and deflate
	//   4. Redirect to IDP SSO URL with SAMLRequest query parameter
	sp.logger.Info("SAML login initiated")
	http.Error(w, "SAML login not yet implemented — integrate crewjam/saml", http.StatusNotImplemented)
}

// handleLogout handles SAML Single Logout (SLO).
func (sp *ServiceProvider) handleLogout(w http.ResponseWriter, r *http.Request) {
	_ = r
	// TODO: Implement SAML SLO. This involves:
	//   1. Building a LogoutRequest
	//   2. Sending it to the IDP's SLO endpoint
	//   3. Invalidating the local session
	sp.logger.Info("SAML logout initiated")
	http.Error(w, "SAML logout not yet implemented — integrate crewjam/saml", http.StatusNotImplemented)
}

// ProcessAssertion extracts user information from a base64-encoded SAML Response.
//
// TODO: Replace the stub implementation with actual SAML assertion parsing
// and validation using crewjam/saml. The implementation should:
//   - Base64-decode the SAMLResponse
//   - Parse the XML
//   - Validate the signature against the IDP certificate
//   - Check conditions (NotBefore, NotOnOrAfter, Audience)
//   - Extract attributes using sp.cfg.AttributeMap
func (sp *ServiceProvider) ProcessAssertion(ctx context.Context, samlResponse string) (*SAMLUser, error) {
	_ = ctx
	_ = samlResponse

	// TODO: Implement actual SAML assertion parsing with crewjam/saml.
	// The code below is a placeholder that returns an error indicating
	// the real implementation is needed.
	return nil, fmt.Errorf("saml: assertion processing not implemented — integrate crewjam/saml library")
}

// spMetadata represents the SP metadata XML document (SAML 2.0).
type spMetadata struct {
	XMLName          xml.Name          `xml:"urn:oasis:names:tc:SAML:2.0:metadata EntityDescriptor"`
	EntityID         string            `xml:"entityID,attr"`
	SPSSODescriptor  spSSODescriptor   `xml:"SPSSODescriptor"`
}

type spSSODescriptor struct {
	XMLName                    xml.Name                    `xml:"urn:oasis:names:tc:SAML:2.0:metadata SPSSODescriptor"`
	ProtocolSupportEnumeration string                      `xml:"protocolSupportEnumeration,attr"`
	AuthnRequestsSigned        bool                        `xml:"AuthnRequestsSigned,attr"`
	WantAssertionsSigned       bool                        `xml:"WantAssertionsSigned,attr"`
	NameIDFormats              []nameIDFormat               `xml:"NameIDFormat"`
	AssertionConsumerServices  []assertionConsumerService   `xml:"AssertionConsumerService"`
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

// GenerateMetadata produces the SP metadata XML document.
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
