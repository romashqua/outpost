package observability

import (
	"log/slog"
	"testing"
)

func TestSanitizeSyslogValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special characters",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "double quotes",
			input: `say "hello"`,
			want:  `say \"hello\"`,
		},
		{
			name:  "backslashes",
			input: `path\to\file`,
			want:  `path\\to\\file`,
		},
		{
			name:  "closing brackets",
			input: "data]end",
			want:  `data\]end`,
		},
		{
			name:  "newlines",
			input: "line1\nline2",
			want:  `line1\nline2`,
		},
		{
			name:  "carriage returns",
			input: "line1\rline2",
			want:  `line1\rline2`,
		},
		{
			name:  "mixed special characters",
			input: "user \"admin\"\nip=10.0.0.1]done\\",
			want:  `user \"admin\"\nip=10.0.0.1\]done\\`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "backslash then quote",
			input: `\"`,
			want:  `\\\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSyslogValue(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeSyslogValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSIEMExporter_Configure(t *testing.T) {
	exporter := NewSIEMExporter(nil, slog.Default())

	// Initially empty
	exporter.mu.RLock()
	if exporter.webhookURL != "" {
		t.Errorf("initial webhookURL = %q, want empty", exporter.webhookURL)
	}
	if exporter.syslogAddr != "" {
		t.Errorf("initial syslogAddr = %q, want empty", exporter.syslogAddr)
	}
	exporter.mu.RUnlock()

	// Configure both
	exporter.Configure("https://siem.example.com/webhook", "syslog.example.com:514")

	exporter.mu.RLock()
	if exporter.webhookURL != "https://siem.example.com/webhook" {
		t.Errorf("webhookURL = %q, want %q", exporter.webhookURL, "https://siem.example.com/webhook")
	}
	if exporter.syslogAddr != "syslog.example.com:514" {
		t.Errorf("syslogAddr = %q, want %q", exporter.syslogAddr, "syslog.example.com:514")
	}
	exporter.mu.RUnlock()

	// Reconfigure to disable syslog
	exporter.Configure("https://siem.example.com/webhook", "")

	exporter.mu.RLock()
	if exporter.syslogAddr != "" {
		t.Errorf("syslogAddr after reconfigure = %q, want empty", exporter.syslogAddr)
	}
	exporter.mu.RUnlock()
}

func TestSIEMExporter_SetHMACSecret(t *testing.T) {
	exporter := NewSIEMExporter(nil, slog.Default())

	exporter.SetHMACSecret("test-secret-key")

	exporter.mu.RLock()
	if exporter.hmacSecret != "test-secret-key" {
		t.Errorf("hmacSecret = %q, want %q", exporter.hmacSecret, "test-secret-key")
	}
	exporter.mu.RUnlock()
}

func TestNewSIEMExporter(t *testing.T) {
	logger := slog.Default()
	exporter := NewSIEMExporter(nil, logger)

	if exporter == nil {
		t.Fatal("NewSIEMExporter returned nil")
	}
	if exporter.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
	if exporter.httpClient.Timeout.Seconds() != 10 {
		t.Errorf("httpClient.Timeout = %v, want 10s", exporter.httpClient.Timeout)
	}
	if exporter.lastID != 0 {
		t.Errorf("lastID = %d, want 0", exporter.lastID)
	}
}

func TestSIEMExporter_ExportToWebhook_NoURL(t *testing.T) {
	exporter := NewSIEMExporter(nil, slog.Default())

	// With no webhook URL configured, ExportToWebhook should be a no-op.
	err := exporter.ExportToWebhook(t.Context(), AuditEntry{
		Action:   "test.action",
		Resource: "test.resource",
	})
	if err != nil {
		t.Errorf("ExportToWebhook with no URL: unexpected error: %v", err)
	}
}

func TestSIEMExporter_ExportToSyslog_NoAddr(t *testing.T) {
	exporter := NewSIEMExporter(nil, slog.Default())

	// With no syslog address configured, ExportToSyslog should be a no-op.
	err := exporter.ExportToSyslog(AuditEntry{
		Action:   "test.action",
		Resource: "test.resource",
	})
	if err != nil {
		t.Errorf("ExportToSyslog with no address: unexpected error: %v", err)
	}
}

func TestSIEMExporter_ExportEvent_NothingConfigured(t *testing.T) {
	exporter := NewSIEMExporter(nil, slog.Default())

	// With nothing configured, ExportEvent should succeed silently.
	err := exporter.ExportEvent(t.Context(), AuditEntry{
		Action:   "test.action",
		Resource: "test.resource",
	})
	if err != nil {
		t.Errorf("ExportEvent with nothing configured: unexpected error: %v", err)
	}
}
