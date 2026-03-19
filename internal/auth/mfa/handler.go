package mfa

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/romashqua/outpost/internal/auth"
)

// Handler provides HTTP endpoints for MFA management.
type Handler struct {
	mgr      *Manager
	webauthn *WebAuthnStore
	ceremony *WebAuthnCeremony // nil if WebAuthn ceremony not configured
}

// NewHandler creates a new MFA Handler.
func NewHandler(mgr *Manager, webauthn *WebAuthnStore, opts ...func(*Handler)) *Handler {
	h := &Handler{mgr: mgr, webauthn: webauthn}
	for _, o := range opts {
		o(h)
	}
	return h
}

// WithCeremony sets the WebAuthn ceremony handler for full attestation/assertion support.
func WithCeremony(c *WebAuthnCeremony) func(*Handler) {
	return func(h *Handler) { h.ceremony = c }
}

// Routes returns a chi.Router with all MFA endpoints mounted.
// All routes assume the request has already passed through auth middleware.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/status", h.getStatus)

	r.Post("/totp/setup", h.setupTOTP)
	r.Post("/totp/verify", h.verifyTOTP)
	r.Delete("/totp", h.disableTOTP)

	r.Post("/backup-codes", h.generateBackupCodes)
	r.Post("/backup-codes/verify", h.verifyBackupCode)

	r.Get("/webauthn/credentials", h.listWebAuthnCredentials)
	r.Post("/webauthn/credentials", h.registerWebAuthnCredential)
	r.Delete("/webauthn/credentials/{id}", h.deleteWebAuthnCredential)

	// WebAuthn ceremony endpoints (registration).
	r.Post("/webauthn/register/begin", h.beginWebAuthnRegistration)
	r.Post("/webauthn/register/finish", h.finishWebAuthnRegistration)

	// WebAuthn ceremony endpoints (login/assertion) — used during MFA step.
	r.Post("/webauthn/login/begin", h.beginWebAuthnLogin)
	r.Post("/webauthn/login/finish", h.finishWebAuthnLogin)

	return r
}

// getStatus returns the current MFA status for the authenticated user.
func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	status, err := h.mgr.GetUserMFAStatus(r.Context(), claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get MFA status")
		return
	}

	writeJSON(w, http.StatusOK, status)
}

type setupTOTPRequest struct {
	Issuer string `json:"issuer"`
}

type setupTOTPResponse struct {
	Secret  string `json:"secret"`
	QRURL   string `json:"qr_url"`
	QRImage string `json:"qr_image"`
}

func (h *Handler) setupTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req setupTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Issuer == "" {
		req.Issuer = "Outpost VPN"
	}

	secret, qrURL, qrImage, err := h.mgr.EnableTOTP(r.Context(), claims.UserID, req.Issuer)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to setup TOTP")
		return
	}

	writeJSON(w, http.StatusOK, setupTOTPResponse{Secret: secret, QRURL: qrURL, QRImage: qrImage})
}

type codeRequest struct {
	Code string `json:"code"`
}

func (h *Handler) verifyTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req codeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Code == "" {
		respondError(w, http.StatusBadRequest, "code is required")
		return
	}

	valid, err := h.mgr.VerifyTOTP(r.Context(), claims.UserID, req.Code)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TOTP verification failed")
		return
	}
	if !valid {
		writeJSON(w, http.StatusOK, map[string]bool{"valid": false})
		return
	}

	if err := h.mgr.SetMFAEnabled(r.Context(), claims.UserID, true); err != nil {
		respondError(w, http.StatusInternalServerError, "TOTP verified but failed to enable MFA flag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

func (h *Handler) disableTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	if err := h.mgr.DisableTOTP(r.Context(), claims.UserID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to disable TOTP")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) generateBackupCodes(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	codes, err := h.mgr.GenerateBackupCodes(r.Context(), claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate backup codes")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]string{"codes": codes})
}

func (h *Handler) verifyBackupCode(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req codeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Code == "" {
		respondError(w, http.StatusBadRequest, "code is required")
		return
	}

	valid, err := h.mgr.ValidateBackupCode(r.Context(), claims.UserID, req.Code)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "backup code verification failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}

func (h *Handler) listWebAuthnCredentials(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	creds, err := h.webauthn.GetCredentials(r.Context(), claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list credentials")
		return
	}

	writeJSON(w, http.StatusOK, creds)
}

// registerWebAuthnRequest is the legacy request body for registering a credential (bypasses ceremony).
type registerWebAuthnRequest struct {
	CredentialID []byte `json:"credential_id"`
	PublicKey    []byte `json:"public_key"`
	Name         string `json:"name"`
}

func (h *Handler) registerWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	var req registerWebAuthnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.CredentialID) == 0 || len(req.PublicKey) == 0 {
		respondError(w, http.StatusBadRequest, "credential_id and public_key are required")
		return
	}

	cred := WebAuthnCredential{
		UserID:       claims.UserID,
		CredentialID: req.CredentialID,
		PublicKey:    req.PublicKey,
		Name:         req.Name,
	}

	if err := h.webauthn.RegisterCredential(r.Context(), cred); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to register credential")
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) deleteWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		respondError(w, http.StatusBadRequest, "credential id is required")
		return
	}

	if err := h.webauthn.DeleteCredentialForUser(r.Context(), credID, claims.UserID); err != nil {
		respondError(w, http.StatusNotFound, "credential not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- WebAuthn Ceremony Endpoints ---

type beginRegisterRequest struct {
	Name string `json:"name"` // optional friendly name for the credential
}

// beginWebAuthnRegistration starts the WebAuthn registration ceremony.
// Returns PublicKeyCredentialCreationOptions for navigator.credentials.create().
func (h *Handler) beginWebAuthnRegistration(w http.ResponseWriter, r *http.Request) {
	if h.ceremony == nil {
		respondError(w, http.StatusServiceUnavailable, "WebAuthn not configured")
		return
	}
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	options, err := h.ceremony.BeginRegistration(r.Context(), claims.UserID, claims.Username, claims.Username)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to begin registration: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, options)
}

type finishRegisterRequest struct {
	Name string `json:"name"` // friendly name for the credential
}

// finishWebAuthnRegistration completes the WebAuthn registration ceremony.
// The request body is the authenticator's attestation response.
func (h *Handler) finishWebAuthnRegistration(w http.ResponseWriter, r *http.Request) {
	if h.ceremony == nil {
		respondError(w, http.StatusServiceUnavailable, "WebAuthn not configured")
		return
	}
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	// Get credential name from query param (body is consumed by webauthn library).
	credName := r.URL.Query().Get("name")
	if credName == "" {
		credName = "Security Key"
	}

	user, err := h.ceremony.BuildUser(r.Context(), claims.UserID, claims.Username, claims.Username)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to build user")
		return
	}

	session, err := h.ceremony.LoadSession(r.Context(), claims.UserID, "register")
	if err != nil {
		respondError(w, http.StatusBadRequest, "no pending registration or session expired")
		return
	}

	credential, err := h.ceremony.GetWebAuthn().FinishRegistration(user, *session, r)
	if err != nil {
		respondError(w, http.StatusBadRequest, "registration verification failed: "+err.Error())
		return
	}

	// Store the verified credential.
	dbCred := WebAuthnCredential{
		UserID:       claims.UserID,
		CredentialID: credential.ID,
		PublicKey:    credential.PublicKey,
		SignCount:    int64(credential.Authenticator.SignCount),
		Name:         credName,
	}
	if err := h.webauthn.RegisterCredential(r.Context(), dbCred); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store credential")
		return
	}

	// Enable MFA on user if not already.
	if err := h.mgr.SetMFAEnabled(r.Context(), claims.UserID, true); err != nil {
		respondError(w, http.StatusInternalServerError, "credential stored but failed to enable MFA")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok", "name": credName})
}

// beginWebAuthnLogin starts the WebAuthn assertion ceremony.
// This is called during MFA verification — requires an MFA token (not full auth).
// For simplicity, we accept both JWT auth and an mfa_token query param.
func (h *Handler) beginWebAuthnLogin(w http.ResponseWriter, r *http.Request) {
	if h.ceremony == nil {
		respondError(w, http.StatusServiceUnavailable, "WebAuthn not configured")
		return
	}

	// Try to get user from JWT context first (authenticated user).
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	options, err := h.ceremony.BeginLogin(r.Context(), claims.UserID, claims.Username, claims.Username)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, options)
}

// finishWebAuthnLogin completes the WebAuthn assertion ceremony.
// On success, updates the credential sign count.
func (h *Handler) finishWebAuthnLogin(w http.ResponseWriter, r *http.Request) {
	if h.ceremony == nil {
		respondError(w, http.StatusServiceUnavailable, "WebAuthn not configured")
		return
	}

	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	user, err := h.ceremony.BuildUser(r.Context(), claims.UserID, claims.Username, claims.Username)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to build user")
		return
	}

	session, err := h.ceremony.LoadSession(r.Context(), claims.UserID, "login")
	if err != nil {
		respondError(w, http.StatusBadRequest, "no pending login or session expired")
		return
	}

	credential, err := h.ceremony.GetWebAuthn().FinishLogin(user, *session, r)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "assertion verification failed: "+err.Error())
		return
	}

	// Update sign count.
	creds, _ := h.webauthn.GetCredentials(r.Context(), claims.UserID)
	for _, c := range creds {
		if bytesEqual(c.CredentialID, credential.ID) {
			_ = h.webauthn.UpdateSignCount(r.Context(), c.ID, int64(credential.Authenticator.SignCount))
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

// bytesEqual compares two byte slices.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Ensure webauthn.Credential is not used directly in type assertion.
var _ webauthn.User = (*webauthnUser)(nil)

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// respondError writes a JSON error response matching the standard API format.
func respondError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message, "message": message})
}
