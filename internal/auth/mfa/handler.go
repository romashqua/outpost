package mfa

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/romashqua/outpost/internal/auth"
)

// Handler provides HTTP endpoints for MFA management.
type Handler struct {
	mgr      *Manager
	webauthn *WebAuthnStore
}

// NewHandler creates a new MFA Handler.
func NewHandler(mgr *Manager, webauthn *WebAuthnStore) *Handler {
	return &Handler{mgr: mgr, webauthn: webauthn}
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

	return r
}

// getStatus returns the current MFA status for the authenticated user.
func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	status, err := h.mgr.GetUserMFAStatus(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, "failed to get MFA status", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// setupTOTPRequest is the request body for TOTP setup.
type setupTOTPRequest struct {
	Issuer string `json:"issuer"`
}

// setupTOTPResponse is the response body for TOTP setup.
type setupTOTPResponse struct {
	Secret string `json:"secret"`
	QRURL  string `json:"qr_url"`
}

// setupTOTP begins TOTP enrollment by generating a secret.
func (h *Handler) setupTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	var req setupTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Issuer == "" {
		req.Issuer = "Outpost VPN"
	}

	secret, qrURL, err := h.mgr.EnableTOTP(r.Context(), claims.UserID, req.Issuer)
	if err != nil {
		http.Error(w, "failed to setup TOTP", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, setupTOTPResponse{Secret: secret, QRURL: qrURL})
}

// codeRequest is a shared request body for code-based verification endpoints.
type codeRequest struct {
	Code string `json:"code"`
}

// verifyTOTP validates a TOTP code and activates TOTP on first success.
func (h *Handler) verifyTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	var req codeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	valid, err := h.mgr.VerifyTOTP(r.Context(), claims.UserID, req.Code)
	if err != nil {
		http.Error(w, "TOTP verification failed", http.StatusInternalServerError)
		return
	}
	if !valid {
		writeJSON(w, http.StatusOK, map[string]bool{"valid": false})
		return
	}

	// Ensure MFA is enabled on the user record once TOTP is verified.
	_ = h.mgr.SetMFAEnabled(r.Context(), claims.UserID, true)

	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

// disableTOTP removes the TOTP configuration for the user.
func (h *Handler) disableTOTP(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	if err := h.mgr.DisableTOTP(r.Context(), claims.UserID); err != nil {
		http.Error(w, "failed to disable TOTP", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// generateBackupCodes creates a fresh set of backup codes.
func (h *Handler) generateBackupCodes(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	codes, err := h.mgr.GenerateBackupCodes(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, "failed to generate backup codes", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string][]string{"codes": codes})
}

// verifyBackupCode validates a single-use backup code.
func (h *Handler) verifyBackupCode(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	var req codeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	valid, err := h.mgr.ValidateBackupCode(r.Context(), claims.UserID, req.Code)
	if err != nil {
		http.Error(w, "backup code verification failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"valid": valid})
}

// listWebAuthnCredentials returns all WebAuthn credentials for the user.
func (h *Handler) listWebAuthnCredentials(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	creds, err := h.webauthn.GetCredentials(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, "failed to list credentials", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, creds)
}

// registerWebAuthnRequest is the request body for registering a credential.
type registerWebAuthnRequest struct {
	CredentialID []byte `json:"credential_id"`
	PublicKey    []byte `json:"public_key"`
	Name         string `json:"name"`
}

// registerWebAuthnCredential stores a new WebAuthn credential.
func (h *Handler) registerWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	var req registerWebAuthnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.CredentialID) == 0 || len(req.PublicKey) == 0 {
		http.Error(w, "credential_id and public_key are required", http.StatusBadRequest)
		return
	}

	cred := WebAuthnCredential{
		UserID:       claims.UserID,
		CredentialID: req.CredentialID,
		PublicKey:    req.PublicKey,
		Name:         req.Name,
	}

	if err := h.webauthn.RegisterCredential(r.Context(), cred); err != nil {
		http.Error(w, "failed to register credential", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// deleteWebAuthnCredential removes a WebAuthn credential by ID.
func (h *Handler) deleteWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		http.Error(w, "credential id is required", http.StatusBadRequest)
		return
	}

	if err := h.webauthn.DeleteCredential(r.Context(), credID); err != nil {
		http.Error(w, "failed to delete credential", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
