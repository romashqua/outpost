package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/romashqua/outpost/internal/wireguard"
)

// GatewayMode turns an outpost-client into a lightweight S2S gateway.
// Instead of connecting as a user device, it registers as a tunnel member
// and creates WireGuard interfaces for site-to-site connectivity.
type GatewayMode struct {
	client    *Client
	logger    *slog.Logger
	tunnelID  string
	gatewayID string
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	stopOnce  sync.Once
	configDir string
}

type s2sConfigResponse struct {
	TunnelID    string        `json:"tunnel_id"`
	TunnelName  string        `json:"tunnel_name"`
	GatewayID   string        `json:"gateway_id"`
	PrivateKey  string        `json:"private_key"`
	Address     string        `json:"address"`
	ListenPort  int           `json:"listen_port"`
	DNS         []string      `json:"dns"`
	Peers       []s2sPeerConf `json:"peers"`
	Routes      []string      `json:"routes"`
}

type s2sPeerConf struct {
	PublicKey  string   `json:"public_key"`
	Endpoint  string   `json:"endpoint"`
	AllowedIPs []string `json:"allowed_ips"`
	Keepalive int      `json:"keepalive"`
}

// NewGatewayMode creates a new S2S gateway mode instance.
func NewGatewayMode(client *Client, tunnelID string, logger *slog.Logger) *GatewayMode {
	if logger == nil {
		logger = slog.Default()
	}
	return &GatewayMode{
		client:    client,
		logger:    logger.With("mode", "gateway", "tunnel", tunnelID),
		tunnelID:  tunnelID,
		configDir: client.configDir,
		stopCh:    make(chan struct{}),
	}
}

// Start registers this client as a S2S tunnel member, fetches the WireGuard
// configuration, and brings up the S2S interface.
func (g *GatewayMode) Start(ctx context.Context) error {
	g.mu.Lock()
	if g.running {
		g.mu.Unlock()
		return fmt.Errorf("gateway mode already running")
	}
	// Mark as running early to prevent concurrent Start calls.
	g.running = true
	g.mu.Unlock()

	g.logger.Info("starting S2S gateway mode")

	// Register as a tunnel member.
	config, err := g.fetchS2SConfig(ctx)
	if err != nil {
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		return fmt.Errorf("fetch S2S config: %w", err)
	}

	g.mu.Lock()
	g.gatewayID = config.GatewayID
	g.mu.Unlock()

	g.logger.Info("registered as S2S member",
		"gateway_id", config.GatewayID,
		"tunnel", config.TunnelName,
		"peers", len(config.Peers),
	)

	// Build WireGuard config.
	wgConfig, err := g.buildWireGuardConfig(config)
	if err != nil {
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		return fmt.Errorf("build wireguard config: %w", err)
	}

	// Write config to disk.
	confPath := filepath.Join(g.configDir, "outpost-s2s.conf")
	if err := os.MkdirAll(g.configDir, 0700); err != nil {
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(confPath, []byte(wgConfig), 0600); err != nil {
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		return fmt.Errorf("write config: %w", err)
	}

	// Bring up the WireGuard interface.
	if err := g.wgUp(confPath); err != nil {
		// Clean up config file containing private key.
		os.Remove(confPath)
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
		return fmt.Errorf("bring up S2S interface: %w", err)
	}

	g.logger.Info("S2S gateway interface is up",
		"routes", config.Routes,
		"peers", len(config.Peers),
	)

	// Start background sync loop.
	go g.syncLoop(ctx)

	return nil
}

// Stop tears down the S2S WireGuard interface and stops the sync loop.
func (g *GatewayMode) Stop() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.running {
		return nil
	}

	g.stopOnce.Do(func() { close(g.stopCh) })
	g.running = false

	confPath := filepath.Join(g.configDir, "outpost-s2s.conf")
	if err := g.wgDown(confPath); err != nil {
		g.logger.Error("failed to tear down S2S interface", "error", err)
		return err
	}

	// Clean up config file containing private key material.
	os.Remove(confPath)

	g.logger.Info("S2S gateway interface stopped")
	return nil
}

// syncLoop periodically checks for config updates and reports health.
func (g *GatewayMode) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-g.stopCh:
			return
		case <-ticker.C:
			g.reportHealth(ctx)
		}
	}
}

// fetchS2SConfig gets the S2S tunnel configuration from the server.
func (g *GatewayMode) fetchS2SConfig(ctx context.Context) (*s2sConfigResponse, error) {
	// Generate a key pair for this gateway instance.
	privKey, err := wireguard.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	// Save private key.
	keyPath := filepath.Join(g.configDir, "s2s_private.key")
	if err := os.MkdirAll(g.configDir, 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(privKey), 0600); err != nil {
		return nil, fmt.Errorf("save private key: %w", err)
	}

	// Fetch the S2S tunnel config using the existing config endpoint.
	// NOTE: The /register-client endpoint does not exist yet.
	// Use the documented config endpoint instead.
	resp, err := g.client.doRequest(ctx, "GET",
		fmt.Sprintf("/api/v1/s2s-tunnels/%s/config/%s", g.tunnelID, g.hostname()), nil, true)
	if err != nil {
		g.logger.Warn("S2S config endpoint not available, skipping registration", "error", err)
		return nil, fmt.Errorf("fetch S2S config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	var config s2sConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Inject our private key.
	config.PrivateKey = privKey

	return &config, nil
}

func (g *GatewayMode) buildWireGuardConfig(config *s2sConfigResponse) (string, error) {
	iface := wireguard.InterfaceConfig{
		PrivateKey: config.PrivateKey,
		Address:    config.Address,
	}
	if config.ListenPort > 0 {
		iface.ListenPort = config.ListenPort
	}
	if len(config.DNS) > 0 {
		iface.DNS = config.DNS
	}

	for _, p := range config.Peers {
		iface.Peers = append(iface.Peers, wireguard.PeerConfig{
			PublicKey:           p.PublicKey,
			Endpoint:            p.Endpoint,
			AllowedIPs:          p.AllowedIPs,
			PersistentKeepalive: p.Keepalive,
		})
	}

	return wireguard.RenderConfig(iface), nil
}

func (g *GatewayMode) reportHealth(_ context.Context) {
	// The /health endpoint for S2S tunnels does not exist yet.
	// Log at debug level and skip the call to avoid noisy errors.
	g.logger.Debug("skipping S2S health report: endpoint not implemented yet")
}

func (g *GatewayMode) hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		h = "outpost-gateway"
	}
	return h
}

func (g *GatewayMode) wgUp(confPath string) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd := exec.Command("wg-quick", "up", confPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.Command("wireguard.exe", "/installtunnelservice", confPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func (g *GatewayMode) wgDown(confPath string) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd := exec.Command("wg-quick", "down", confPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case "windows":
		cmd := exec.Command("wireguard.exe", "/uninstalltunnelservice", "outpost-s2s")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
