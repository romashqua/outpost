package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMailHandler_TestSMTP_NilMailer(t *testing.T) {
	h := NewMailHandler(nil)
	router := h.Routes()

	body := `{"to":"test@example.com"}`
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// Inject admin claims so RequireAdmin middleware passes.
	req = reqWithAdminClaims(req)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMailHandler_TestSMTP_MissingTo(t *testing.T) {
	h := NewMailHandler(nil)
	router := h.Routes()

	body := `{}`
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = reqWithAdminClaims(req)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// With nil mailer, we get 400 "SMTP is not configured" before checking "to"
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
