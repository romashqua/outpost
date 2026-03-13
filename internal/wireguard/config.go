package wireguard

import (
	"fmt"
	"strings"
)

// PeerConfig represents a WireGuard peer in wg-quick configuration format.
type PeerConfig struct {
	PublicKey           string
	AllowedIPs          []string
	Endpoint            string
	PresharedKey        string
	PersistentKeepalive int
}

// InterfaceConfig represents a complete WireGuard interface configuration
// including its peers.
type InterfaceConfig struct {
	PrivateKey string
	Address    string
	ListenPort int
	DNS        []string
	MTU        int
	Peers      []PeerConfig
}

// RenderConfig renders a full wg-quick compatible configuration file from the
// given interface configuration.
func RenderConfig(iface InterfaceConfig) string {
	var b strings.Builder

	b.WriteString("[Interface]\n")
	b.WriteString(fmt.Sprintf("PrivateKey = %s\n", iface.PrivateKey))

	if iface.Address != "" {
		b.WriteString(fmt.Sprintf("Address = %s\n", iface.Address))
	}

	if iface.ListenPort > 0 {
		b.WriteString(fmt.Sprintf("ListenPort = %d\n", iface.ListenPort))
	}

	if len(iface.DNS) > 0 {
		b.WriteString(fmt.Sprintf("DNS = %s\n", strings.Join(iface.DNS, ", ")))
	}

	if iface.MTU > 0 {
		b.WriteString(fmt.Sprintf("MTU = %d\n", iface.MTU))
	}

	for _, peer := range iface.Peers {
		b.WriteString("\n")
		b.WriteString(RenderPeerConfig(peer))
	}

	return b.String()
}

// RenderPeerConfig renders a single [Peer] section in wg-quick format.
func RenderPeerConfig(peer PeerConfig) string {
	var b strings.Builder

	b.WriteString("[Peer]\n")
	b.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))

	if peer.PresharedKey != "" {
		b.WriteString(fmt.Sprintf("PresharedKey = %s\n", peer.PresharedKey))
	}

	if len(peer.AllowedIPs) > 0 {
		b.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", ")))
	}

	if peer.Endpoint != "" {
		b.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
	}

	if peer.PersistentKeepalive > 0 {
		b.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", peer.PersistentKeepalive))
	}

	return b.String()
}
