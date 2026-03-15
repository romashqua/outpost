package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/romashqua/outpost/internal/mail"
)

// MailHandler provides endpoints for email-related operations.
type MailHandler struct {
	mailer *mail.Mailer
}

// NewMailHandler creates a MailHandler with the given mailer instance.
func NewMailHandler(mailer *mail.Mailer) *MailHandler {
	return &MailHandler{mailer: mailer}
}

// Routes returns a chi.Router with mail endpoints mounted.
func (h *MailHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/test", h.testSMTP)
	return r
}

// testSMTP sends a test email to verify SMTP configuration.
// @Summary Test email delivery
// @Description Send a test email to verify SMTP is working.
// @Tags Mail
// @Accept json
// @Produce json
// @Param body body object true "Recipient email" example({"to": "user@example.com"})
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Security BearerAuth
// @Router /mail/test [post]
func (h *MailHandler) testSMTP(w http.ResponseWriter, r *http.Request) {
	if h.mailer == nil {
		respondError(w, http.StatusBadRequest, "SMTP is not configured")
		return
	}

	var body struct {
		To string `json:"to"`
	}
	if err := parseBody(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.To == "" {
		respondError(w, http.StatusBadRequest, "to (email address) is required")
		return
	}

	err := h.mailer.Send(context.Background(), body.To,
		"Outpost VPN - SMTP Test",
		"<h1>SMTP Configuration Test</h1><p>If you are reading this, your SMTP settings are working correctly.</p>")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to send test email: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}
