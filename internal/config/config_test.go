package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear any environment variables that might interfere.
	envVars := []string{
		"OUTPOST_HTTP_ADDR", "OUTPOST_GRPC_ADDR",
		"OUTPOST_DB_HOST", "OUTPOST_DB_PORT", "OUTPOST_DB_NAME", "OUTPOST_DB_USER",
		"OUTPOST_REDIS_ADDR",
		"OUTPOST_WG_INTERFACE", "OUTPOST_WG_LISTEN_PORT", "OUTPOST_WG_MTU",
		"OUTPOST_TOKEN_TTL", "OUTPOST_SESSION_TTL",
		"OUTPOST_LOG_LEVEL", "OUTPOST_LOG_FORMAT",
		"OUTPOST_JWT_SECRET",
		"OUTPOST_SMTP_PORT", "OUTPOST_SMTP_FROM_NAME",
		"OUTPOST_STUN_PORT", "OUTPOST_TURN_PORT", "OUTPOST_TURN_REALM",
	}
	for _, key := range envVars {
		os.Unsetenv(key)
	}

	cfg := Load()

	// Server defaults
	if cfg.Server.HTTPAddr != ":8080" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, ":8080")
	}
	if cfg.Server.GRPCAddr != ":9090" {
		t.Errorf("Server.GRPCAddr = %q, want %q", cfg.Server.GRPCAddr, ":9090")
	}

	// Database defaults
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "localhost")
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5432)
	}
	if cfg.Database.Name != "outpost" {
		t.Errorf("Database.Name = %q, want %q", cfg.Database.Name, "outpost")
	}
	if cfg.Database.SSLMode != "disable" {
		t.Errorf("Database.SSLMode = %q, want %q", cfg.Database.SSLMode, "disable")
	}
	if cfg.Database.MaxConns != 20 {
		t.Errorf("Database.MaxConns = %d, want %d", cfg.Database.MaxConns, 20)
	}
	if cfg.Database.MinConns != 2 {
		t.Errorf("Database.MinConns = %d, want %d", cfg.Database.MinConns, 2)
	}

	// Redis defaults
	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Redis.Addr, "localhost:6379")
	}

	// WireGuard defaults
	if cfg.WireGuard.InterfaceName != "wg0" {
		t.Errorf("WireGuard.InterfaceName = %q, want %q", cfg.WireGuard.InterfaceName, "wg0")
	}
	if cfg.WireGuard.ListenPort != 51820 {
		t.Errorf("WireGuard.ListenPort = %d, want %d", cfg.WireGuard.ListenPort, 51820)
	}
	if cfg.WireGuard.MTU != 1420 {
		t.Errorf("WireGuard.MTU = %d, want %d", cfg.WireGuard.MTU, 1420)
	}

	// Auth defaults
	if cfg.Auth.TokenTTL != 15*time.Minute {
		t.Errorf("Auth.TokenTTL = %v, want %v", cfg.Auth.TokenTTL, 15*time.Minute)
	}
	if cfg.Auth.SessionTTL != 24*time.Hour {
		t.Errorf("Auth.SessionTTL = %v, want %v", cfg.Auth.SessionTTL, 24*time.Hour)
	}
	// JWT secret should be auto-generated (non-empty)
	if cfg.Auth.JWTSecret == "" {
		t.Error("Auth.JWTSecret should be auto-generated when env var is not set")
	}

	// Log defaults
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}

	// LDAP disabled by default
	if cfg.LDAP.Enabled {
		t.Error("LDAP.Enabled should be false by default")
	}

	// SAML disabled by default
	if cfg.SAML.Enabled {
		t.Error("SAML.Enabled should be false by default")
	}

	// NAT disabled by default
	if cfg.NAT.Enabled {
		t.Error("NAT.Enabled should be false by default")
	}
	if cfg.NAT.STUNPort != 3478 {
		t.Errorf("NAT.STUNPort = %d, want %d", cfg.NAT.STUNPort, 3478)
	}

	// SMTP defaults
	if cfg.SMTP.Port != 587 {
		t.Errorf("SMTP.Port = %d, want %d", cfg.SMTP.Port, 587)
	}
	if cfg.SMTP.FromName != "Outpost VPN" {
		t.Errorf("SMTP.FromName = %q, want %q", cfg.SMTP.FromName, "Outpost VPN")
	}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
	// Set overrides
	overrides := map[string]string{
		"OUTPOST_HTTP_ADDR":    ":9999",
		"OUTPOST_DB_HOST":      "db.example.com",
		"OUTPOST_DB_PORT":      "5433",
		"OUTPOST_DB_NAME":      "mydb",
		"OUTPOST_REDIS_ADDR":   "redis.example.com:6380",
		"OUTPOST_WG_INTERFACE": "wg1",
		"OUTPOST_WG_MTU":       "1400",
		"OUTPOST_LOG_LEVEL":    "debug",
		"OUTPOST_LOG_FORMAT":   "text",
		"OUTPOST_TOKEN_TTL":    "30m",
		"OUTPOST_SESSION_TTL":  "48h",
		"OUTPOST_JWT_SECRET":   "my-test-secret",
		"OUTPOST_LDAP_ENABLED": "true",
		"OUTPOST_NAT_ENABLED":  "true",
		"OUTPOST_SMTP_PORT":    "465",
		"OUTPOST_SMTP_TLS":     "true",
	}

	for k, v := range overrides {
		os.Setenv(k, v)
	}
	defer func() {
		for k := range overrides {
			os.Unsetenv(k)
		}
	}()

	cfg := Load()

	if cfg.Server.HTTPAddr != ":9999" {
		t.Errorf("Server.HTTPAddr = %q, want %q", cfg.Server.HTTPAddr, ":9999")
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "db.example.com")
	}
	if cfg.Database.Port != 5433 {
		t.Errorf("Database.Port = %d, want %d", cfg.Database.Port, 5433)
	}
	if cfg.Database.Name != "mydb" {
		t.Errorf("Database.Name = %q, want %q", cfg.Database.Name, "mydb")
	}
	if cfg.Redis.Addr != "redis.example.com:6380" {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Redis.Addr, "redis.example.com:6380")
	}
	if cfg.WireGuard.InterfaceName != "wg1" {
		t.Errorf("WireGuard.InterfaceName = %q, want %q", cfg.WireGuard.InterfaceName, "wg1")
	}
	if cfg.WireGuard.MTU != 1400 {
		t.Errorf("WireGuard.MTU = %d, want %d", cfg.WireGuard.MTU, 1400)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "text")
	}
	if cfg.Auth.TokenTTL != 30*time.Minute {
		t.Errorf("Auth.TokenTTL = %v, want %v", cfg.Auth.TokenTTL, 30*time.Minute)
	}
	if cfg.Auth.SessionTTL != 48*time.Hour {
		t.Errorf("Auth.SessionTTL = %v, want %v", cfg.Auth.SessionTTL, 48*time.Hour)
	}
	if cfg.Auth.JWTSecret != "my-test-secret" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "my-test-secret")
	}
	if !cfg.LDAP.Enabled {
		t.Error("LDAP.Enabled should be true when env is set to 'true'")
	}
	if !cfg.NAT.Enabled {
		t.Error("NAT.Enabled should be true when env is set to 'true'")
	}
	if cfg.SMTP.Port != 465 {
		t.Errorf("SMTP.Port = %d, want %d", cfg.SMTP.Port, 465)
	}
	if !cfg.SMTP.TLS {
		t.Error("SMTP.TLS should be true when env is set to 'true'")
	}
}

func TestEnvInt_InvalidFallsBackToDefault(t *testing.T) {
	os.Setenv("OUTPOST_DB_PORT", "not-a-number")
	defer os.Unsetenv("OUTPOST_DB_PORT")

	got := envInt("OUTPOST_DB_PORT", 5432)
	if got != 5432 {
		t.Errorf("envInt with invalid value = %d, want fallback 5432", got)
	}
}

func TestEnvDuration_InvalidFallsBackToDefault(t *testing.T) {
	os.Setenv("OUTPOST_TOKEN_TTL", "not-a-duration")
	defer os.Unsetenv("OUTPOST_TOKEN_TTL")

	got := envDuration("OUTPOST_TOKEN_TTL", 15*time.Minute)
	if got != 15*time.Minute {
		t.Errorf("envDuration with invalid value = %v, want fallback 15m", got)
	}
}

func TestEnv_EmptyFallsBackToDefault(t *testing.T) {
	os.Unsetenv("OUTPOST_NONEXISTENT_VAR")

	got := env("OUTPOST_NONEXISTENT_VAR", "default-val")
	if got != "default-val" {
		t.Errorf("env with missing var = %q, want %q", got, "default-val")
	}
}

func TestEnvJWTSecret_GeneratesRandomWhenMissing(t *testing.T) {
	os.Unsetenv("OUTPOST_JWT_SECRET")

	s1 := envJWTSecret()
	s2 := envJWTSecret()

	if s1 == "" {
		t.Error("envJWTSecret should return a non-empty string")
	}
	if len(s1) != 64 {
		t.Errorf("envJWTSecret: got length %d, want 64 (32 bytes hex-encoded)", len(s1))
	}
	if s1 == s2 {
		t.Error("envJWTSecret: two calls should produce different random secrets")
	}
}

func TestEnvJWTSecret_UsesEnvVar(t *testing.T) {
	os.Setenv("OUTPOST_JWT_SECRET", "explicit-secret")
	defer os.Unsetenv("OUTPOST_JWT_SECRET")

	got := envJWTSecret()
	if got != "explicit-secret" {
		t.Errorf("envJWTSecret = %q, want %q", got, "explicit-secret")
	}
}
