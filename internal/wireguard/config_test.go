package wireguard

import (
	"strings"
	"testing"
)

func TestRenderPeerConfig_Basic(t *testing.T) {
	peer := PeerConfig{
		PublicKey:           "abc123pubkey=",
		AllowedIPs:          []string{"10.0.0.0/24"},
		Endpoint:            "vpn.example.com:51820",
		PresharedKey:        "psk123=",
		PersistentKeepalive: 25,
	}

	got := RenderPeerConfig(peer)

	expects := []string{
		"[Peer]",
		"PublicKey = abc123pubkey=",
		"PresharedKey = psk123=",
		"AllowedIPs = 10.0.0.0/24",
		"Endpoint = vpn.example.com:51820",
		"PersistentKeepalive = 25",
	}
	for _, want := range expects {
		if !strings.Contains(got, want) {
			t.Errorf("RenderPeerConfig output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestRenderPeerConfig_EmptyOptionalFields(t *testing.T) {
	peer := PeerConfig{
		PublicKey: "onlypubkey=",
	}

	got := RenderPeerConfig(peer)

	if !strings.Contains(got, "PublicKey = onlypubkey=") {
		t.Errorf("expected PublicKey line, got:\n%s", got)
	}
	if strings.Contains(got, "PresharedKey") {
		t.Errorf("empty PresharedKey should be omitted, got:\n%s", got)
	}
	if strings.Contains(got, "AllowedIPs") {
		t.Errorf("empty AllowedIPs should be omitted, got:\n%s", got)
	}
	if strings.Contains(got, "Endpoint") {
		t.Errorf("empty Endpoint should be omitted, got:\n%s", got)
	}
	if strings.Contains(got, "PersistentKeepalive") {
		t.Errorf("zero PersistentKeepalive should be omitted, got:\n%s", got)
	}
}

func TestRenderPeerConfig_MultipleAllowedIPs(t *testing.T) {
	peer := PeerConfig{
		PublicKey:   "key=",
		AllowedIPs:  []string{"10.0.0.0/24", "192.168.1.0/24", "fd00::/64"},
	}

	got := RenderPeerConfig(peer)

	want := "AllowedIPs = 10.0.0.0/24, 192.168.1.0/24, fd00::/64"
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in output, got:\n%s", want, got)
	}
}

func TestRenderConfig_FullConfig(t *testing.T) {
	iface := InterfaceConfig{
		PrivateKey: "privkey123=",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
		DNS:        []string{"1.1.1.1", "8.8.8.8"},
		MTU:        1420,
		Peers: []PeerConfig{
			{
				PublicKey:   "peer1key=",
				AllowedIPs:  []string{"10.0.0.2/32"},
				Endpoint:    "1.2.3.4:51820",
			},
		},
	}

	got := RenderConfig(iface)

	expects := []string{
		"[Interface]",
		"PrivateKey = privkey123=",
		"Address = 10.0.0.1/24",
		"ListenPort = 51820",
		"DNS = 1.1.1.1, 8.8.8.8",
		"MTU = 1420",
		"[Peer]",
		"PublicKey = peer1key=",
		"AllowedIPs = 10.0.0.2/32",
		"Endpoint = 1.2.3.4:51820",
	}
	for _, want := range expects {
		if !strings.Contains(got, want) {
			t.Errorf("RenderConfig output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestRenderConfig_MinimalConfig(t *testing.T) {
	iface := InterfaceConfig{
		PrivateKey: "privkey=",
	}

	got := RenderConfig(iface)

	if !strings.Contains(got, "[Interface]") {
		t.Error("missing [Interface] header")
	}
	if !strings.Contains(got, "PrivateKey = privkey=") {
		t.Error("missing PrivateKey")
	}
	if strings.Contains(got, "Address") {
		t.Error("empty Address should be omitted")
	}
	if strings.Contains(got, "ListenPort") {
		t.Error("zero ListenPort should be omitted")
	}
	if strings.Contains(got, "DNS") {
		t.Error("empty DNS should be omitted")
	}
	if strings.Contains(got, "MTU") {
		t.Error("zero MTU should be omitted")
	}
	if strings.Contains(got, "[Peer]") {
		t.Error("no peers should produce no [Peer] sections")
	}
}

func TestRenderConfig_MultiplePeers(t *testing.T) {
	iface := InterfaceConfig{
		PrivateKey: "privkey=",
		Peers: []PeerConfig{
			{PublicKey: "peer1=", AllowedIPs: []string{"10.0.0.1/32"}},
			{PublicKey: "peer2=", AllowedIPs: []string{"10.0.0.2/32"}},
			{PublicKey: "peer3=", AllowedIPs: []string{"10.0.0.3/32"}},
		},
	}

	got := RenderConfig(iface)

	peerCount := strings.Count(got, "[Peer]")
	if peerCount != 3 {
		t.Errorf("expected 3 [Peer] sections, got %d\n%s", peerCount, got)
	}

	for _, key := range []string{"peer1=", "peer2=", "peer3="} {
		if !strings.Contains(got, "PublicKey = "+key) {
			t.Errorf("missing peer with key %s", key)
		}
	}
}
