package mail

import (
	"log/slog"
	"strings"
	"testing"
)

func TestNewMailer(t *testing.T) {
	cfg := Config{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		FromAddress: "noreply@example.com",
		FromName:    "Test",
		Username:    "user",
		Password:    "pass",
		TLS:         true,
	}
	logger := slog.Default()

	m := NewMailer(cfg, logger)
	if m == nil {
		t.Fatal("NewMailer returned nil")
	}
	if m.cfg.SMTPHost != "smtp.example.com" {
		t.Errorf("cfg.SMTPHost = %q, want %q", m.cfg.SMTPHost, "smtp.example.com")
	}
	if m.cfg.SMTPPort != 587 {
		t.Errorf("cfg.SMTPPort = %d, want %d", m.cfg.SMTPPort, 587)
	}
	if !m.cfg.TLS {
		t.Error("cfg.TLS should be true")
	}
}

func TestRenderTemplate_MFACode(t *testing.T) {
	result, err := renderTemplate("mfa", mfaCodeTemplate, map[string]string{
		"Code": "123456",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "123456") {
		t.Error("rendered MFA template should contain the code")
	}
	if !strings.Contains(result, "Verification Code") {
		t.Error("rendered MFA template should contain heading")
	}
}

func TestRenderTemplate_EnrollmentInvite(t *testing.T) {
	result, err := renderTemplate("enroll", enrollmentInviteTemplate, map[string]string{
		"EnrollURL":    "https://vpn.example.com/enroll?token=abc",
		"InstanceName": "Acme Corp VPN",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "https://vpn.example.com/enroll?token=abc") {
		t.Error("rendered enrollment template should contain the enroll URL")
	}
	if !strings.Contains(result, "Acme Corp VPN") {
		t.Error("rendered enrollment template should contain the instance name")
	}
}

func TestRenderTemplate_PasswordReset(t *testing.T) {
	result, err := renderTemplate("reset", passwordResetTemplate, map[string]string{
		"ResetURL": "https://vpn.example.com/reset?token=xyz",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "https://vpn.example.com/reset?token=xyz") {
		t.Error("rendered reset template should contain the reset URL")
	}
	if !strings.Contains(result, "Password Reset") {
		t.Error("rendered reset template should contain heading")
	}
}

func TestRenderTemplate_Welcome(t *testing.T) {
	result, err := renderTemplate("welcome", welcomeTemplate, map[string]string{
		"Username":     "alice",
		"InstanceName": "Outpost Demo",
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "alice") {
		t.Error("rendered welcome template should contain the username")
	}
	if !strings.Contains(result, "Outpost Demo") {
		t.Error("rendered welcome template should contain the instance name")
	}
}

func TestRenderTemplate_DeviceConfig(t *testing.T) {
	configText := "[Interface]\nPrivateKey = abc123\nAddress = 10.0.0.2/32"
	result, err := renderTemplate("device_config", deviceConfigTemplate, map[string]string{
		"DeviceName": "laptop-alice",
		"Config":     configText,
	})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(result, "laptop-alice") {
		t.Error("rendered device config template should contain device name")
	}
	if !strings.Contains(result, "10.0.0.2/32") {
		t.Error("rendered device config template should contain config address")
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	_, err := renderTemplate("bad", "{{.Missing", nil)
	if err == nil {
		t.Error("renderTemplate with invalid template syntax should return error")
	}
}

func TestRenderTemplate_ExecutionError(t *testing.T) {
	// Passing a data type that does not satisfy the template (e.g., calling a method
	// on nil) can cause execution errors. Here we use a template that references a
	// field on a nil map.
	tmpl := "{{.Nonexistent}}"
	// With a nil map, Execute succeeds with "<no value>" in Go templates.
	result, err := renderTemplate("test", tmpl, map[string]string{})
	if err != nil {
		t.Fatalf("renderTemplate: unexpected error: %v", err)
	}
	if strings.Contains(result, "Nonexistent") {
		t.Error("result should not literally contain 'Nonexistent'")
	}
}

func TestConfigValidation_EmptyHost(t *testing.T) {
	cfg := Config{
		SMTPHost:    "",
		SMTPPort:    587,
		FromAddress: "noreply@example.com",
	}
	m := NewMailer(cfg, slog.Default())
	// Mailer is created regardless; validation happens at send time.
	if m == nil {
		t.Fatal("NewMailer should not return nil even with empty host")
	}
	if m.cfg.SMTPHost != "" {
		t.Errorf("cfg.SMTPHost = %q, want empty", m.cfg.SMTPHost)
	}
}

func TestConfigValidation_ZeroPort(t *testing.T) {
	cfg := Config{
		SMTPHost:    "smtp.example.com",
		SMTPPort:    0,
		FromAddress: "noreply@example.com",
	}
	m := NewMailer(cfg, slog.Default())
	if m.cfg.SMTPPort != 0 {
		t.Errorf("cfg.SMTPPort = %d, want 0", m.cfg.SMTPPort)
	}
}
