package gateway

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/romashqua/outpost/internal/config"
	"google.golang.org/grpc/metadata"
)

// --- NewAgent ---

func TestNewAgent(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Gateway: config.Gateway{
			Token:         "test-token",
			CoreAddr:      "localhost:50051",
			InterfaceName: "wg-test",
		},
	}

	agent, err := NewAgent(cfg, logger)
	if err != nil {
		t.Fatalf("NewAgent() error: %v", err)
	}
	if agent == nil {
		t.Fatal("NewAgent() returned nil agent")
	}
	if agent.cfg != cfg {
		t.Error("agent.cfg does not match provided config")
	}
	if agent.wg != nil {
		t.Error("expected wg to be nil initially")
	}
	if agent.conn != nil {
		t.Error("expected conn to be nil initially")
	}
}

// --- authContext ---

func TestAgent_AuthContext(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Gateway: config.Gateway{
			Token: "my-secret-token",
		},
	}

	agent, _ := NewAgent(cfg, logger)
	ctx := agent.authContext(context.Background())

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata in context")
	}
	authVals := md.Get("authorization")
	if len(authVals) != 1 {
		t.Fatalf("expected 1 authorization value, got %d", len(authVals))
	}
	if authVals[0] != "Bearer my-secret-token" {
		t.Errorf("authorization = %q, want 'Bearer my-secret-token'", authVals[0])
	}
}

// --- getClient ---

func TestAgent_GetClient_Nil(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{}, logger)
	client := agent.getClient()
	if client != nil {
		t.Error("expected nil client before connect")
	}
}

// --- closeConn ---

func TestAgent_CloseConn_NilSafe(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{}, logger)
	// Should not panic when conn is nil.
	agent.closeConn()
	if agent.conn != nil {
		t.Error("conn should be nil after closeConn")
	}
	if agent.client != nil {
		t.Error("client should be nil after closeConn")
	}
}

// --- buildTransportCredentials ---

func TestAgent_BuildTransportCredentials_Insecure(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{
		Gateway: config.Gateway{
			TLSEnabled: false,
		},
	}, logger)

	creds, err := agent.buildTransportCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
	// Insecure credentials report "insecure" as security protocol.
	info := creds.Info()
	if info.SecurityProtocol != "insecure" {
		t.Errorf("expected 'insecure' security protocol, got %q", info.SecurityProtocol)
	}
}

func TestAgent_BuildTransportCredentials_TLS(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{
		Gateway: config.Gateway{
			TLSEnabled:            true,
			TLSInsecureSkipVerify: true,
		},
	}, logger)

	creds, err := agent.buildTransportCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds == nil {
		t.Fatal("expected non-nil credentials")
	}
	info := creds.Info()
	if info.SecurityProtocol != "tls" {
		t.Errorf("expected 'tls' security protocol, got %q", info.SecurityProtocol)
	}
}

func TestAgent_BuildTransportCredentials_BadCertFile(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{
		Gateway: config.Gateway{
			TLSEnabled:  true,
			TLSCertFile: "/nonexistent/cert.pem",
			TLSKeyFile:  "/nonexistent/key.pem",
		},
	}, logger)

	_, err := agent.buildTransportCredentials()
	if err == nil {
		t.Fatal("expected error with nonexistent cert files")
	}
}

func TestAgent_BuildTransportCredentials_BadCAFile(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{
		Gateway: config.Gateway{
			TLSEnabled: true,
			TLSCAFile:  "/nonexistent/ca.pem",
		},
	}, logger)

	_, err := agent.buildTransportCredentials()
	if err == nil {
		t.Fatal("expected error with nonexistent CA file")
	}
}

func TestAgent_BuildTransportCredentials_InvalidCAContent(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create a temp file with invalid PEM content.
	tmpFile, err := os.CreateTemp("", "bad-ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("not a valid PEM certificate")
	tmpFile.Close()

	agent, _ := NewAgent(&config.Config{
		Gateway: config.Gateway{
			TLSEnabled: true,
			TLSCAFile:  tmpFile.Name(),
		},
	}, logger)

	_, err = agent.buildTransportCredentials()
	if err == nil {
		t.Fatal("expected error with invalid CA content")
	}
}

// --- fetchConfig without client ---

func TestAgent_FetchConfig_NoClient(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{}, logger)

	_, err := agent.fetchConfig(context.Background())
	if err == nil {
		t.Fatal("expected error when no client is available")
	}
}

// --- Concurrent access to getClient/closeConn ---

func TestAgent_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	agent, _ := NewAgent(&config.Config{}, logger)

	done := make(chan struct{})
	// Run concurrent getClient and closeConn calls to verify no race.
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = agent.getClient()
			agent.closeConn()
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
