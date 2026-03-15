package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// peerHealthStatus represents the health of a single S2S tunnel peer
// based on the WireGuard last handshake time.
type peerHealthStatus struct {
	PublicKey     string `json:"public_key"`
	Status        string `json:"status"` // HEALTHY, DEGRADED, DOWN
	LastHandshake int64  `json:"last_handshake_unix"`
	RxBytes       int64  `json:"rx_bytes"`
	TxBytes       int64  `json:"tx_bytes"`
	Endpoint      string `json:"endpoint,omitempty"`
}

// s2sHealthReport is sent to core to report tunnel peer health.
type s2sHealthReport struct {
	TunnelID  string             `json:"tunnel_id"`
	GatewayID string             `json:"gateway_id"`
	Peers     []peerHealthStatus `json:"peers"`
	Timestamp int64              `json:"timestamp"`
}

// classifyHealth determines peer health from the WireGuard last handshake time.
//   - HEALTHY:  handshake within the last 2 minutes
//   - DEGRADED: handshake within the last 5 minutes
//   - DOWN:     no handshake or older than 5 minutes
func classifyHealth(lastHandshake time.Time) string {
	if lastHandshake.IsZero() {
		return "DOWN"
	}
	age := time.Since(lastHandshake)
	switch {
	case age < 2*time.Minute:
		return "HEALTHY"
	case age < 5*time.Minute:
		return "DEGRADED"
	default:
		return "DOWN"
	}
}

// reportHealth collects WireGuard peer stats for the S2S interface and
// reports health status to core. Health is determined by the last handshake
// time from wgctrl, which is more reliable than ICMP and requires no
// special permissions.
func (g *GatewayMode) reportHealth(ctx context.Context) {
	g.mu.RLock()
	tunnelID := g.tunnelID
	gatewayID := g.gatewayID
	g.mu.RUnlock()

	// Collect WireGuard peer stats by parsing "wg show" output.
	// We use wg show because the client does not have direct wgctrl access
	// (that lives in the gateway package); the S2S interface is managed
	// via wg-quick, so "wg show" is the portable way to read stats.
	peers, err := g.collectWGPeerStats()
	if err != nil {
		g.logger.Warn("failed to collect WireGuard peer stats for health report", "error", err)
		return
	}

	if len(peers) == 0 {
		g.logger.Debug("no S2S peers found, skipping health report")
		return
	}

	report := s2sHealthReport{
		TunnelID:  tunnelID,
		GatewayID: gatewayID,
		Peers:     peers,
		Timestamp: time.Now().Unix(),
	}

	body, err := json.Marshal(report)
	if err != nil {
		g.logger.Warn("failed to marshal health report", "error", err)
		return
	}

	resp, err := g.client.doRequest(ctx, "POST",
		fmt.Sprintf("/api/v1/s2s-tunnels/%s/health", tunnelID), body, true)
	if err != nil {
		g.logger.Debug("failed to send S2S health report", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		healthy := 0
		degraded := 0
		down := 0
		for _, p := range peers {
			switch p.Status {
			case "HEALTHY":
				healthy++
			case "DEGRADED":
				degraded++
			case "DOWN":
				down++
			}
		}
		g.logger.Info("S2S health report sent",
			"peers", len(peers),
			"healthy", healthy,
			"degraded", degraded,
			"down", down,
		)
	} else {
		g.logger.Debug("S2S health report rejected by server", "status", resp.StatusCode)
	}
}

// collectWGPeerStats runs "wg show outpost-s2s dump" and parses the output
// to extract per-peer stats including the last handshake timestamp.
// The dump format (tab-separated, one peer per line after the interface line):
//
//	public-key  preshared-key  endpoint  allowed-ips  latest-handshake  transfer-rx  transfer-tx  persistent-keepalive
func (g *GatewayMode) collectWGPeerStats() ([]peerHealthStatus, error) {
	// Determine the interface name from the config file name.
	ifaceName := "outpost-s2s"

	cmd := exec.Command("wg", "show", ifaceName, "dump")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("wg show %s dump: %w (stderr: %s)", ifaceName, err, stderr.String())
	}

	var peers []peerHealthStatus
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	// First line is the interface line, skip it.
	for i, line := range lines {
		if i == 0 {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		pubKey := fields[0]
		endpoint := fields[2]
		// fields[4] = latest-handshake (unix timestamp, 0 if never)
		// fields[5] = transfer-rx (bytes)
		// fields[6] = transfer-tx (bytes)

		var handshakeUnix int64
		fmt.Sscanf(fields[4], "%d", &handshakeUnix)

		var rxBytes, txBytes int64
		fmt.Sscanf(fields[5], "%d", &rxBytes)
		fmt.Sscanf(fields[6], "%d", &txBytes)

		var lastHandshake time.Time
		if handshakeUnix > 0 {
			lastHandshake = time.Unix(handshakeUnix, 0)
		}

		status := classifyHealth(lastHandshake)

		peers = append(peers, peerHealthStatus{
			PublicKey:     pubKey,
			Status:        status,
			LastHandshake: handshakeUnix,
			RxBytes:       rxBytes,
			TxBytes:       txBytes,
			Endpoint:      endpoint,
		})
	}

	return peers, nil
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
