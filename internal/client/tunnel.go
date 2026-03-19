package client

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/romashqua/outpost/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl"
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

	// In-process wireguard-go device (macOS).
	wgDevice  wgDeviceCloser
	tunDevice tunDeviceCloser

	// HA gateway failover.
	gateways       []GatewayEndpoint // all available gateways, sorted by priority
	currentGateway int               // index into gateways slice
	allowedIPs     []string          // for peer reconfiguration on failover

	// Callbacks for state changes.
	onStateChange func(TunnelState)
}

// wgDeviceCloser is the interface for an in-process wireguard-go device.
type wgDeviceCloser interface {
	Close()
}

// tunDeviceCloser is the interface for a TUN device.
type tunDeviceCloser interface {
	Close() error
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
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onStateChange = fn
}

func (tm *TunnelManager) setState(s TunnelState) {
	tm.mu.Lock()
	old := tm.state
	tm.state = s
	cb := tm.onStateChange
	tm.mu.Unlock()

	if old != s {
		tm.logger.Info("tunnel state changed", "from", old.String(), "to", s.String())
		if cb != nil {
			cb(s)
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
		"gateways", len(enrollment.Gateways),
	)

	// Store gateways for HA failover.
	tm.mu.Lock()
	tm.gateways = enrollment.Gateways
	tm.currentGateway = 0
	tm.allowedIPs = enrollment.AllowedIPs
	tm.mu.Unlock()

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

	// Bring up tunnel using built-in userspace WireGuard (no wg-quick dependency).
	if err := tm.userspaceUp(iface); err != nil {
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

	if err := tm.userspaceDown(); err != nil {
		return fmt.Errorf("bring down tunnel: %w", err)
	}

	// Clean up any leftover config files.
	os.Remove(filepath.Join(tm.configDir, tm.ifaceName+".conf"))

	tm.setState(TunnelDisconnected)
	return nil
}

// RunSessionLoop keeps the session alive, reports posture, monitors gateway
// health for HA failover, and handles MFA re-authentication.
func (tm *TunnelManager) RunSessionLoop(ctx context.Context, postureInterval time.Duration) {
	postureTicker := time.NewTicker(postureInterval)
	refreshTicker := time.NewTicker(20 * time.Minute)     // Refresh token before 24h expiry.
	healthTicker := time.NewTicker(15 * time.Second)       // Check gateway health for failover.
	defer postureTicker.Stop()
	defer refreshTicker.Stop()
	defer healthTicker.Stop()

	// Send initial posture report immediately on connect.
	if tm.State() == TunnelConnected {
		if err := tm.client.ReportPosture(ctx); err != nil {
			tm.logger.Warn("initial posture report failed", "error", err)
		}
	}

	var noHandshakeCount int

	for {
		select {
		case <-ctx.Done():
			return
		case <-postureTicker.C:
			if tm.State() == TunnelConnected {
				if err := tm.client.ReportPosture(ctx); err != nil {
					tm.logger.Warn("posture report failed", "error", err)
				}
			}
		case <-refreshTicker.C:
			if err := tm.client.RefreshSession(ctx); err != nil {
				tm.logger.Warn("session refresh failed, MFA may be required", "error", err)
				tm.setState(TunnelMFARequired)
			}
		case <-healthTicker.C:
			if tm.State() != TunnelConnected {
				noHandshakeCount = 0
				continue
			}
			tm.mu.RLock()
			gwCount := len(tm.gateways)
			tm.mu.RUnlock()
			if gwCount <= 1 {
				continue // No failover targets.
			}

			// Check last handshake via WireGuard stats.
			lastHandshake := tm.getLastHandshake()
			if lastHandshake.IsZero() || time.Since(lastHandshake) > 45*time.Second {
				noHandshakeCount++
				if noHandshakeCount >= 3 {
					// 3 checks * 15s = 45s without handshake — failover.
					tm.failoverToNextGateway()
					noHandshakeCount = 0
				}
			} else {
				noHandshakeCount = 0
			}
		}
	}
}

// failoverToNextGateway switches the WireGuard peer to the next available gateway.
func (tm *TunnelManager) failoverToNextGateway() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if len(tm.gateways) <= 1 {
		return
	}

	oldIdx := tm.currentGateway
	tm.currentGateway = (tm.currentGateway + 1) % len(tm.gateways)
	newGW := tm.gateways[tm.currentGateway]

	tm.logger.Warn("gateway failover",
		"from", tm.gateways[oldIdx].Endpoint,
		"to", newGW.Endpoint,
		"gateway_id", newGW.ID,
	)

	// Reconfigure WireGuard peer with new endpoint.
	if tm.wgDevice != nil {
		iface := wireguard.InterfaceConfig{
			Peers: []wireguard.PeerConfig{
				{
					PublicKey:           newGW.ServerKey,
					AllowedIPs:          tm.allowedIPs,
					Endpoint:            newGW.Endpoint,
					PersistentKeepalive: 25,
				},
			},
		}
		if err := tm.applyPeerUpdate(iface.Peers[0]); err != nil {
			tm.logger.Error("failed to apply failover peer update", "error", err)
		}
	}
}

// getLastHandshake queries wgctrl for the most recent peer handshake time.
// Returns zero time if unavailable.
func (tm *TunnelManager) getLastHandshake() time.Time {
	client, err := wgctrl.New()
	if err != nil {
		return time.Time{}
	}
	defer client.Close()

	dev, err := client.Device(tm.ifaceName)
	if err != nil {
		return time.Time{}
	}

	var latest time.Time
	for _, p := range dev.Peers {
		if p.LastHandshakeTime.After(latest) {
			latest = p.LastHandshakeTime
		}
	}
	return latest
}

// applyPeerUpdate reconfigures the WireGuard tunnel with a new peer endpoint.
// On macOS it uses IPC (UAPI) on the in-process wireguard-go device.
// On Linux it uses wgctrl to update the kernel WireGuard interface.
func (tm *TunnelManager) applyPeerUpdate(peer wireguard.PeerConfig) error {
	tm.logger.Info("applying peer update for failover",
		"endpoint", peer.Endpoint,
		"pubkey", peer.PublicKey[:8]+"...",
	)

	// macOS: reconfigure via IPC on the in-process device.
	if ipcDev, ok := tm.wgDevice.(interface{ IpcSet(string) error }); ok {
		cfg := wireguard.InterfaceConfig{
			Peers: []wireguard.PeerConfig{peer},
		}
		ipc, err := buildIpcPeerConfig(cfg.Peers[0])
		if err != nil {
			return fmt.Errorf("build ipc peer config: %w", err)
		}
		return ipcDev.IpcSet(ipc)
	}

	// Linux: reconfigure via wgctrl.
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wgctrl.New: %w", err)
	}
	defer client.Close()

	wgCfg, err := buildWgctrlPeerConfig(peer)
	if err != nil {
		return err
	}
	return client.ConfigureDevice(tm.ifaceName, *wgCfg)
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

// writeConfigFile writes a WireGuard config to the config directory (used by wg-quick fallback).
func writeConfigFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600)
}
