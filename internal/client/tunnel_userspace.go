package client

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/romashqua/outpost/internal/wireguard"
)

// userspaceUp brings up a WireGuard tunnel using wgctrl (userspace-compatible).
// On macOS this works via wireguard-go utun; on Linux it can use either kernel or userspace.
// This avoids the need for wg-quick.
func (tm *TunnelManager) userspaceUp(cfg wireguard.InterfaceConfig) error {
	ifaceName := tm.ifaceName

	// On macOS, create the utun interface via wireguard-go if available,
	// otherwise fall back to creating it directly.
	if runtime.GOOS == "darwin" {
		// Try wireguard-go first (creates utun interface).
		// wireguard-go uses "utun" prefix on macOS.
		cmd := exec.Command("wireguard-go", ifaceName)
		if out, err := cmd.CombinedOutput(); err != nil {
			// If wireguard-go is not available, try wg-quick as last resort.
			tm.logger.Warn("wireguard-go not available, trying wg-quick", "error", err, "output", string(out))
			return tm.wgQuickFallback(cfg)
		}
		// Wait a moment for the interface to be created.
		time.Sleep(500 * time.Millisecond)
	} else if runtime.GOOS == "linux" {
		// On Linux, create the interface via ip link.
		if out, err := exec.Command("ip", "link", "add", ifaceName, "type", "wireguard").CombinedOutput(); err != nil {
			outStr := strings.TrimSpace(string(out))
			if !strings.Contains(outStr, "File exists") {
				return fmt.Errorf("create interface: %w (%s)", err, outStr)
			}
		}
	}

	// Configure the interface via wgctrl.
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("wgctrl.New: %w", err)
	}
	defer client.Close()

	privKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	var peers []wgtypes.PeerConfig
	for _, p := range cfg.Peers {
		pubKey, err := wgtypes.ParseKey(p.PublicKey)
		if err != nil {
			tm.logger.Warn("skip peer: invalid pubkey", "error", err)
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

	wgCfg := wgtypes.Config{
		PrivateKey: &privKey,
		Peers:      peers,
	}

	if err := client.ConfigureDevice(ifaceName, wgCfg); err != nil {
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

// userspaceDown tears down the userspace WireGuard tunnel.
func (tm *TunnelManager) userspaceDown() error {
	ifaceName := tm.ifaceName

	switch runtime.GOOS {
	case "darwin":
		// On macOS, removing the utun interface: find and kill wireguard-go process.
		// The interface is removed when the process exits.
		_ = exec.Command("bash", "-c", fmt.Sprintf(
			"pgrep -f 'wireguard-go %s' | xargs kill 2>/dev/null", ifaceName)).Run()
		// Also try the socket-based shutdown.
		_ = exec.Command("rm", "-f", fmt.Sprintf("/var/run/wireguard/%s.sock", ifaceName)).Run()
	case "linux":
		_ = exec.Command("ip", "link", "del", ifaceName).Run()
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
		// macOS: ifconfig utun7 inet 10.129.0.2 10.129.0.2 netmask 255.255.255.0
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

// wgQuickFallback tries to use wg-quick as a last resort.
func (tm *TunnelManager) wgQuickFallback(cfg wireguard.InterfaceConfig) error {
	// Write temp config for wg-quick.
	configText := wireguard.RenderConfig(cfg)
	configPath := tm.configDir + "/" + tm.ifaceName + ".conf"
	if err := writeConfigFile(configPath, configText); err != nil {
		return err
	}
	return exec.Command("wg-quick", "up", configPath).Run()
}
