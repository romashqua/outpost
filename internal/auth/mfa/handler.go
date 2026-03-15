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

// setupTOTPRequest is the request body for TOTP setup.
type setupTOTPRequest struct {
	Issuer string `json:"issuer"`
}

// setupTOTPResponse is the response body for TOTP setup.
type setupTOTPResponse struct {
	Secret  string `json:"secret"`
	QRURL   string `json:"qr_url"`
	QRImage string `json:"qr_image"`
}

// setupTOTP begins TOTP enrollment by generating a secret.
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

// codeRequest is a shared request body for code-based verification endpoints.
type codeRequest struct {
	Code string `json:"code"`
}

// verifyTOTP validates a TOTP code and activates TOTP on first success.
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

	// Ensure MFA is enabled on the user record once TOTP is verified.
	if err := h.mgr.SetMFAEnabled(r.Context(), claims.UserID, true); err != nil {
		respondError(w, http.StatusInternalServerError, "TOTP verified but failed to enable MFA flag")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

// disableTOTP removes the TOTP configuration for the user.
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

// generateBackupCodes creates a fresh set of backup codes.
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

// verifyBackupCode validates a single-use backup code.
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

// listWebAuthnCredentials returns all WebAuthn credentials for the user.
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

// deleteWebAuthnCredential removes a WebAuthn credential by ID.
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
