package client

import (
	"context"
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

// TunnelState represents the current state of the VPN tunnel.
type TunnelState int

const (
	TunnelDisconnected TunnelState = iota
	TunnelConnecting
	TunnelConnected
	TunnelReconnecting
	TunnelMFARequired
)

func (s TunnelState) String() string {
	switch s {
	case TunnelDisconnected:
		return "disconnected"
	case TunnelConnecting:
		return "connecting"
	case TunnelConnected:
		return "connected"
	case TunnelReconnecting:
		return "reconnecting"
	case TunnelMFARequired:
		return "mfa_required"
	default:
		return "unknown"
	}
}

// TunnelManager manages the WireGuard tunnel lifecycle.
type TunnelManager struct {
	client    *Client
	logger    *slog.Logger
	state     TunnelState
	mu        sync.RWMutex
	ifaceName string
	configDir string

	// Callbacks for state changes.
	onStateChange func(TunnelState)
}

// NewTunnelManager creates a new tunnel manager.
func NewTunnelManager(client *Client, logger *slog.Logger) *TunnelManager {
	return &TunnelManager{
		client:    client,
		logger:    logger,
		state:     TunnelDisconnected,
		ifaceName: "outpost0",
		configDir: client.configDir,
	}
}

// State returns the current tunnel state.
func (tm *TunnelManager) State() TunnelState {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.state
}

// OnStateChange registers a callback for tunnel state changes.
func (tm *TunnelManager) OnStateChange(fn func(TunnelState)) {
	tm.onStateChange = fn
}

func (tm *TunnelManager) setState(s TunnelState) {
	tm.mu.Lock()
	old := tm.state
	tm.state = s
	tm.mu.Unlock()

	if old != s {
		tm.logger.Info("tunnel state changed", "from", old.String(), "to", s.String())
		if tm.onStateChange != nil {
			tm.onStateChange(s)
		}
	}
}

// Connect establishes the VPN tunnel. This is the main entry point that handles
// the full flow: login -> MFA -> enroll -> configure WireGuard -> connect.
func (tm *TunnelManager) Connect(ctx context.Context, networkID string) error {
	tm.setState(TunnelConnecting)

	// Generate or load WireGuard keys.
	privateKey, publicKey, err := tm.ensureKeys()
	if err != nil {
		tm.setState(TunnelDisconnected)
		return fmt.Errorf("ensure keys: %w", err)
	}

	// Enroll device (or re-enroll if needed).
	hostname, _ := os.Hostname()
	enrollment, err := tm.client.Enroll(ctx, publicKey, hostname)
	if err != nil {
		tm.setState(TunnelDisconnected)
		return fmt.Errorf("enroll device: %w", err)
	}

	tm.logger.Info("device enrolled",
		"device_id", enrollment.DeviceID,
		"address", enrollment.Address,
	)

	// Build WireGuard configuration.
	iface := wireguard.InterfaceConfig{
		PrivateKey: privateKey,
		Address:    enrollment.Address,
		DNS:        enrollment.DNS,
	}

	// Add network peers.
	if len(enrollment.Networks) > 0 {
		for _, net := range enrollment.Networks {
			iface.Peers = append(iface.Peers, wireguard.PeerConfig{
				PublicKey:           net.ServerKey,
				AllowedIPs:          net.AllowedIPs,
				Endpoint:            net.Endpoint,
				PersistentKeepalive: 25,
			})
		}
	} else {
		// Fallback: use enrollment-level config.
		iface.Peers = append(iface.Peers, wireguard.PeerConfig{
			PublicKey:           enrollment.ServerKey,
			AllowedIPs:          enrollment.AllowedIPs,
			Endpoint:            enrollment.Endpoint,
			PersistentKeepalive: 25,
		})
	}

	// Write config and bring up interface.
	configPath := filepath.Join(tm.configDir, tm.ifaceName+".conf")
	if err := os.WriteFile(configPath, []byte(wireguard.RenderConfig(iface)), 0600); err != nil {
		tm.setState(TunnelDisconnected)
		return fmt.Errorf("write config: %w", err)
	}

	if err := tm.wgUp(configPath); err != nil {
		tm.setState(TunnelDisconnected)
		return fmt.Errorf("bring up tunnel: %w", err)
	}

	tm.setState(TunnelConnected)
	return nil
}

// Disconnect tears down the VPN tunnel.
func (tm *TunnelManager) Disconnect() error {
	if tm.State() == TunnelDisconnected {
		return nil
	}

	configPath := filepath.Join(tm.configDir, tm.ifaceName+".conf")
	if err := tm.wgDown(configPath); err != nil {
		return fmt.Errorf("bring down tunnel: %w", err)
	}

	tm.setState(TunnelDisconnected)
	return nil
}

// RunSessionLoop keeps the session alive, reports posture, and handles
// MFA re-authentication when the session expires.
func (tm *TunnelManager) RunSessionLoop(ctx context.Context, postureInterval time.Duration) {
	postureTicker := time.NewTicker(postureInterval)
	refreshTicker := time.NewTicker(20 * time.Minute) // Refresh token before 24h expiry.
	defer postureTicker.Stop()
	defer refreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-postureTicker.C:
			if tm.State() == TunnelConnected {
				if err := tm.client.ReportPosture(ctx); err != nil {
					tm.logger.Warn("posture report failed", "error", err)
					// If 401/403 — MFA session expired, tunnel will be torn down server-side.
				}
			}
		case <-refreshTicker.C:
			if err := tm.client.RefreshSession(ctx); err != nil {
				tm.logger.Warn("session refresh failed, MFA may be required", "error", err)
				tm.setState(TunnelMFARequired)
			}
		}
	}
}

// --- Key Management ---

func (tm *TunnelManager) ensureKeys() (privateKey, publicKey string, err error) {
	keyPath := filepath.Join(tm.configDir, "private.key")

	data, err := os.ReadFile(keyPath)
	if err == nil {
		privateKey = string(data)
		publicKey, err = wireguard.PublicKey(privateKey)
		if err == nil {
			return privateKey, publicKey, nil
		}
	}

	// Generate new key pair.
	privateKey, err = wireguard.GeneratePrivateKey()
	if err != nil {
		return "", "", fmt.Errorf("generate private key: %w", err)
	}

	publicKey, err = wireguard.PublicKey(privateKey)
	if err != nil {
		return "", "", fmt.Errorf("derive public key: %w", err)
	}

	if err := os.MkdirAll(tm.configDir, 0700); err != nil {
		return "", "", err
	}

	if err := os.WriteFile(keyPath, []byte(privateKey), 0600); err != nil {
		return "", "", fmt.Errorf("save private key: %w", err)
	}

	return privateKey, publicKey, nil
}

// --- WireGuard Interface Control ---

func (tm *TunnelManager) wgUp(configPath string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("wg-quick", "up", configPath).Run()
	case "darwin":
		// macOS: use wireguard-go userspace + wg-quick from wireguard-tools.
		return exec.Command("wg-quick", "up", configPath).Run()
	case "windows":
		// Windows: use wireguard.exe tunnel service.
		return exec.Command("wireguard.exe", "/installtunnelservice", configPath).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func (tm *TunnelManager) wgDown(configPath string) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return exec.Command("wg-quick", "down", configPath).Run()
	case "windows":
		return exec.Command("wireguard.exe", "/uninstalltunnelservice", tm.ifaceName).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
