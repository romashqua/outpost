package gateway

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// WGManager manages a WireGuard interface via wgctrl.
type WGManager struct {
	mu     sync.Mutex
	iface  string
	client *wgctrl.Client
	logger *slog.Logger
}

// NewWGManager creates a manager for the given interface name.
func NewWGManager(iface string, logger *slog.Logger) (*WGManager, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("wgctrl.New: %w", err)
	}
	return &WGManager{
		iface:  iface,
		client: client,
		logger: logger.With("component", "wg_manager", "iface", iface),
	}, nil
}

// Close releases wgctrl resources.
func (m *WGManager) Close() error {
	return m.client.Close()
}

// AddPeer adds or updates a peer on the WireGuard interface.
func (m *WGManager) AddPeer(pubkey string, allowedIPs []string, endpoint string, keepalive int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, err := wgtypes.ParseKey(pubkey)
	if err != nil {
		return fmt.Errorf("parse pubkey: %w", err)
	}

	var nets []net.IPNet
	for _, cidr := range allowedIPs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			m.logger.Warn("skipping invalid allowed IP", "cidr", cidr, "error", err)
			continue
		}
		nets = append(nets, *ipnet)
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey:         key,
		ReplaceAllowedIPs: true,
		AllowedIPs:        nets,
	}

	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err == nil {
			peerCfg.Endpoint = addr
		}
	}

	err = m.client.ConfigureDevice(m.iface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	})
	if err != nil {
		return fmt.Errorf("add peer: %w", err)
	}

	pubkeyPreview := pubkey
	if len(pubkeyPreview) >= 8 {
		pubkeyPreview = pubkeyPreview[:8] + "..."
	}
	m.logger.Info("peer added", "pubkey", pubkeyPreview, "allowed_ips", allowedIPs)
	return nil
}

// RemovePeer removes a peer from the WireGuard interface.
func (m *WGManager) RemovePeer(pubkey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key, err := wgtypes.ParseKey(pubkey)
	if err != nil {
		return fmt.Errorf("parse pubkey: %w", err)
	}

	err = m.client.ConfigureDevice(m.iface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: key,
				Remove:   true,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("remove peer: %w", err)
	}

	pubkeyPreview := pubkey
	if len(pubkeyPreview) >= 8 {
		pubkeyPreview = pubkeyPreview[:8] + "..."
	}
	m.logger.Info("peer removed", "pubkey", pubkeyPreview)
	return nil
}

// GetPeerStats returns current peer statistics from the interface.
func (m *WGManager) GetPeerStats() ([]wgtypes.Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dev, err := m.client.Device(m.iface)
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}
	return dev.Peers, nil
}
