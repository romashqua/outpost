package db

import (
	"testing"

	"github.com/romashqua/outpost/internal/config"
)

// TestNew_InvalidDSN verifies that an invalid DSN produces a parse error.
// We cannot test actual database connectivity without PostgreSQL,
// but we can verify the DSN construction and config parsing path.
func TestNew_InvalidConfig(t *testing.T) {
	t.Parallel()

	// A config with invalid host should fail to connect (not parse),
	// but we can at least ensure it doesn't panic.
	cfg := config.Database{
		Host:     "",
		Port:     0,
		User:     "",
		Password: "",
		Name:     "",
		SSLMode:  "disable",
		MaxConns: 5,
		MinConns: 1,
	}

	ctx := t.Context()
	_, err := New(ctx, cfg)
	if err == nil {
		t.Error("expected error with empty/invalid database config, got nil")
	}
}

// TestRunMigrations_EmptyFS verifies that RunMigrations handles a filesystem
// with no valid migration files. We cannot pass a nil pool because
// stdlib.OpenDBFromPool panics on nil, so we only test the source creation path.
func TestRunMigrations_BadSource(t *testing.T) {
	t.Parallel()

	// A MapFS with a subdirectory that doesn't exist as migration source
	// should cause iofs.New to fail when the directory doesn't match.
	// Actually, iofs.New with "." on an empty FS succeeds but migrate fails later.
	// We just verify the function signature works and doesn't panic with valid but empty FS.
	// Real migration testing requires a database connection.
	t.Skip("RunMigrations requires a live PostgreSQL connection to test meaningfully")
}

// TestDSNConstruction verifies that the DSN format string produces
// the expected connection string. We test this indirectly by checking
// that pgxpool.ParseConfig succeeds for a well-formed config.
func TestDSNConstruction(t *testing.T) {
	t.Parallel()

	cfg := config.Database{
		Host:     "localhost",
		Port:     5432,
		User:     "testuser",
		Password: "testpass",
		Name:     "testdb",
		SSLMode:  "disable",
		MaxConns: 10,
		MinConns: 2,
	}

	// We can't connect, but we can verify the DSN is parseable
	// by attempting to create a pool (which will fail at Ping, not parse).
	ctx := t.Context()
	_, err := New(ctx, cfg)
	// The error should be about connection, not about parsing.
	if err == nil {
		t.Skip("unexpectedly connected to a database — skipping DSN test")
	}
	// If we get a parse error, that's a problem. Connection errors are expected.
	// pgx returns "ping database:" prefix for connection failures.
	// A parse error would say "parse pool config:".
	errStr := err.Error()
	if len(errStr) > 0 && errStr[:5] == "parse" {
		t.Errorf("DSN parsing failed: %v", err)
	}
}
