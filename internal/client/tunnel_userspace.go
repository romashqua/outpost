package client

import (
	"encoding/hex"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	wg "github.com/romashqua/outpost/internal/wireguard"
)

// userspaceUp brings up a WireGuard tunnel.
// On macOS it uses the wireguard-go library in-process (no external binaries).
// On Linux it uses kernel WireGuard + wgctrl.
func (tm *TunnelManager) userspaceUp(cfg wg.InterfaceConfig) error {
	if runtime.GOOS == "darwin" {
		return tm.darwinUp(cfg)
	}
	return tm.linuxUp(cfg)
}

// darwinUp creates a WireGuard tunnel on macOS using the wireguard-go library
// in-process. No external wireguard-go binary or wg-quick needed.
func (tm *TunnelManager) darwinUp(cfg wg.InterfaceConfig) error {
	// Create TUN device. Passing "utun" lets macOS pick the next available utun number.
	tunDev, err := tun.CreateTUN("utun", 1420)
	if err != nil {
		return fmt.Errorf("create tun: %w", err)
	}

	// Get the actual interface name assigned by the kernel (e.g. "utun7").
	actualName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return fmt.Errorf("get tun name: %w", err)
	}
	tm.ifaceName = actualName
	tm.logger.Info("created TUN interface", "name", actualName)

	// Create in-process wireguard-go device.
	logger := device.NewLogger(device.LogLevelError, fmt.Sprintf("wg(%s) ", actualName))
	wgDev := device.NewDevice(tunDev, conn.NewStdNetBind(), logger)

	// Configure via IPC (hex-encoded keys, UAPI format).
	ipcConf, err := buildIpcConfig(cfg)
	if err != nil {
		wgDev.Close()
		tunDev.Close()
		return fmt.Errorf("build ipc config: %w", err)
	}

	if err := wgDev.IpcSet(ipcConf); err != nil {
		wgDev.Close()
		tunDev.Close()
		return fmt.Errorf("ipc set: %w", err)
	}

	if err := wgDev.Up(); err != nil {
		wgDev.Close()
		tunDev.Close()
		return fmt.Errorf("device up: %w", err)
	}

	// Store references for cleanup.
	tm.wgDevice = wgDev
	tm.tunDevice = tunDev

	// Assign address to the interface.
	if cfg.Address != "" {
		if err := tm.assignAddress(actualName, cfg.Address); err != nil {
			return fmt.Errorf("assign address: %w", err)
		}
	}

	// Bring interface up.
	if err := tm.setInterfaceUp(actualName); err != nil {
		return fmt.Errorf("set interface up: %w", err)
	}

	// Add routes for AllowedIPs.
	for _, p := range cfg.Peers {
		for _, cidr := range p.AllowedIPs {
			_ = tm.addRoute(actualName, cidr)
		}
	}

	// Set DNS if configured.
	if len(cfg.DNS) > 0 {
		_ = tm.setDNS(cfg.DNS)
	}

	return nil
}

// linuxUp creates a WireGuard tunnel on Linux using kernel WireGuard + wgctrl.
func (tm *TunnelManager) linuxUp(cfg wg.InterfaceConfig) error {
	ifaceName := tm.ifaceName

	// Create the interface via ip link.
	if out, err := exec.Command("ip", "link", "add", ifaceName, "type", "wireguard").CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if !strings.Contains(outStr, "File exists") {
			return fmt.Errorf("create interface: %w (%s)", err, outStr)
		}
	}

	// Configure the interface via wgctrl.
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wgctrl.New: %w", err)
	}
	defer client.Close()

	wgCfg, err := buildWgctrlConfig(cfg)
	if err != nil {
		return err
	}

	if err := client.ConfigureDevice(ifaceName, *wgCfg); err != nil {
		return fmt.Errorf("configure device: %w", err)
	}

	// Assign address.
	if cfg.Address != "" {
		if err := tm.assignAddress(ifaceName, cfg.Address); err != nil {
			return fmt.Errorf("assign address: %w", err)
		}
	}

	// Bring interface up.
	if err := tm.setInterfaceUp(ifaceName); err != nil {
		return fmt.Errorf("set interface up: %w", err)
	}

	// Add routes for AllowedIPs.
	for _, p := range cfg.Peers {
		for _, cidr := range p.AllowedIPs {
			_ = tm.addRoute(ifaceName, cidr)
		}
	}

	// Set DNS if configured.
	if len(cfg.DNS) > 0 {
		_ = tm.setDNS(cfg.DNS)
	}

	return nil
}

// buildIpcConfig formats a WireGuard config in UAPI IPC format (hex-encoded keys).
func buildIpcConfig(cfg wg.InterfaceConfig) (string, error) {
	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\n", hex.EncodeToString(privKey[:]))

	for _, p := range cfg.Peers {
		pubKey, err := wgtypes.ParseKey(p.PublicKey)
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(pubKey[:]))

		if p.Endpoint != "" {
			fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
		}

		for _, cidr := range p.AllowedIPs {
			fmt.Fprintf(&b, "allowed_ip=%s\n", cidr)
		}

		if p.PersistentKeepalive > 0 {
			fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", p.PersistentKeepalive)
		}
	}

	return b.String(), nil
}

// buildIpcPeerConfig formats a single peer update in UAPI IPC format (for macOS failover).
func buildIpcPeerConfig(p wg.PeerConfig) (string, error) {
	pubKey, err := wgtypes.ParseKey(p.PublicKey)
	if err != nil {
		return "", fmt.Errorf("parse public key: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(pubKey[:]))
	if p.Endpoint != "" {
		fmt.Fprintf(&b, "endpoint=%s\n", p.Endpoint)
	}
	for _, cidr := range p.AllowedIPs {
		fmt.Fprintf(&b, "allowed_ip=%s\n", cidr)
	}
	if p.PersistentKeepalive > 0 {
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\n", p.PersistentKeepalive)
	}
	return b.String(), nil
}

// buildWgctrlPeerConfig converts a single peer to wgctrl format (for Linux failover).
func buildWgctrlPeerConfig(p wg.PeerConfig) (*wgtypes.Config, error) {
	pubKey, err := wgtypes.ParseKey(p.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}

	var allowedIPs []net.IPNet
	for _, cidr := range p.AllowedIPs {
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		allowedIPs = append(allowedIPs, *ipnet)
	}

	peerCfg := wgtypes.PeerConfig{
		PublicKey:         pubKey,
		UpdateOnly:        true,
		ReplaceAllowedIPs: true,
		AllowedIPs:        allowedIPs,
	}
	if p.Endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", p.Endpoint)
		if err == nil {
			peerCfg.Endpoint = addr
		}
	}
	if p.PersistentKeepalive > 0 {
		ka := time.Duration(p.PersistentKeepalive) * time.Second
		peerCfg.PersistentKeepaliveInterval = &ka
	}

	return &wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerCfg},
	}, nil
}

// buildWgctrlConfig converts our config to wgctrl format (for Linux kernel WireGuard).
func buildWgctrlConfig(cfg wg.InterfaceConfig) (*wgtypes.Config, error) {
	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	var peers []wgtypes.PeerConfig
	for _, p := range cfg.Peers {
		pubKey, err := wgtypes.ParseKey(p.PublicKey)
		if err != nil {
			continue
		}

		var allowedIPs []net.IPNet
		for _, cidr := range p.AllowedIPs {
			_, ipnet, err := net.ParseCIDR(cidr)
			if err != nil {
				continue
			}
			allowedIPs = append(allowedIPs, *ipnet)
		}

		peerCfg := wgtypes.PeerConfig{
			PublicKey:         pubKey,
			ReplaceAllowedIPs: true,
			AllowedIPs:        allowedIPs,
		}

		if p.Endpoint != "" {
			addr, err := net.ResolveUDPAddr("udp", p.Endpoint)
			if err == nil {
				peerCfg.Endpoint = addr
			}
		}

		if p.PersistentKeepalive > 0 {
			ka := time.Duration(p.PersistentKeepalive) * time.Second
			peerCfg.PersistentKeepaliveInterval = &ka
		}

		peers = append(peers, peerCfg)
	}

	return &wgtypes.Config{
		PrivateKey: &privKey,
		Peers:      peers,
	}, nil
}

// userspaceDown tears down the userspace WireGuard tunnel.
func (tm *TunnelManager) userspaceDown() error {
	// Close in-process wireguard-go device (macOS).
	if tm.wgDevice != nil {
		tm.wgDevice.Close()
		tm.wgDevice = nil
	}
	if tm.tunDevice != nil {
		tm.tunDevice.Close()
		tm.tunDevice = nil
	}

	switch runtime.GOOS {
	case "darwin":
		// TUN interface is removed when the device is closed.
		// Clean up any leftover socket files.
		_ = exec.Command("rm", "-f", fmt.Sprintf("/var/run/wireguard/%s.sock", tm.ifaceName)).Run()
	case "linux":
		_ = exec.Command("ip", "link", "del", tm.ifaceName).Run()
	}

	return nil
}

func (tm *TunnelManager) assignAddress(iface, addr string) error {
	switch runtime.GOOS {
	case "darwin":
		// Parse CIDR to get IP and mask.
		ip, ipnet, err := net.ParseCIDR(addr)
		if err != nil {
			return err
		}
		// macOS: ifconfig utun7 inet 10.10.0.2 10.10.0.2 netmask 255.255.255.0
		mask := net.IP(ipnet.Mask).String()
		out, err := exec.Command("ifconfig", iface, "inet", ip.String(), ip.String(), "netmask", mask).CombinedOutput()
		if err != nil {
			return fmt.Errorf("ifconfig: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		out, err := exec.Command("ip", "addr", "add", addr, "dev", iface).CombinedOutput()
		if err != nil {
			outStr := strings.TrimSpace(string(out))
			if !strings.Contains(outStr, "File exists") {
				return fmt.Errorf("ip addr add: %w (%s)", err, outStr)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (tm *TunnelManager) setInterfaceUp(iface string) error {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ifconfig", iface, "up").CombinedOutput()
		if err != nil {
			return fmt.Errorf("ifconfig up: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "linux":
		out, err := exec.Command("ip", "link", "set", iface, "up").CombinedOutput()
		if err != nil {
			return fmt.Errorf("ip link set up: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

func (tm *TunnelManager) addRoute(iface, cidr string) error {
	switch runtime.GOOS {
	case "darwin":
		// route add -net 10.129.0.0/24 -interface utun7
		out, err := exec.Command("route", "add", "-net", cidr, "-interface", iface).CombinedOutput()
		if err != nil {
			outStr := strings.TrimSpace(string(out))
			if !strings.Contains(outStr, "already in table") {
				tm.logger.Warn("failed to add route", "cidr", cidr, "error", outStr)
			}
		}
		return nil
	case "linux":
		out, err := exec.Command("ip", "route", "add", cidr, "dev", iface).CombinedOutput()
		if err != nil {
			outStr := strings.TrimSpace(string(out))
			if !strings.Contains(outStr, "File exists") {
				tm.logger.Warn("failed to add route", "cidr", cidr, "error", outStr)
			}
		}
		return nil
	default:
		return nil
	}
}

func (tm *TunnelManager) setDNS(dns []string) error {
	switch runtime.GOOS {
	case "darwin":
		// macOS: use networksetup or scutil to set DNS.
		// This is best-effort; the user may need to set DNS manually.
		for _, d := range dns {
			tm.logger.Info("DNS server (set manually if needed)", "dns", d)
		}
		return nil
	case "linux":
		// On Linux, use resolvconf if available.
		_ = exec.Command("bash", "-c",
			fmt.Sprintf("echo '%s' | resolvconf -a %s 2>/dev/null",
				formatResolvConf(dns), tm.ifaceName)).Run()
		return nil
	default:
		return nil
	}
}

func formatResolvConf(dns []string) string {
	var lines []string
	for _, d := range dns {
		lines = append(lines, "nameserver "+d)
	}
	return strings.Join(lines, "\n")
}
