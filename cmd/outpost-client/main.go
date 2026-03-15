package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/romashqua/outpost/internal/client"
	"github.com/romashqua/outpost/pkg/version"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	serverURL := os.Getenv("OUTPOST_SERVER")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	c := client.NewClient(serverURL)

	switch os.Args[1] {
	case "connect":
		cmdConnect(c, logger)
	case "disconnect":
		cmdDisconnect(c, logger)
	case "gateway":
		cmdGateway(c, logger)
	case "login":
		cmdLogin(c, logger)
	case "status":
		cmdStatus(c, logger)
	case "posture":
		cmdPosture()
	case "version":
		fmt.Printf("outpost-client %s\n", version.Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`outpost-client — Outpost VPN Client with MFA support

Usage:
  outpost-client <command> [options]

Commands:
  connect      Connect to VPN (login + MFA + tunnel)
  disconnect   Disconnect from VPN
  gateway      Run as S2S gateway (gateway <tunnel-id>)
  login        Authenticate without connecting
  status       Show connection status
  posture      Show device security posture
  version      Show version

Environment:
  OUTPOST_SERVER   Server URL (default: http://localhost:8080)
  OUTPOST_USER     Username for non-interactive login
  OUTPOST_PASS     Password for non-interactive login`)
}

func cmdConnect(c *client.Client, logger *slog.Logger) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Try loading existing token.
	if err := c.LoadToken(); err != nil {
		// Need to login.
		if err := doLogin(ctx, c); err != nil {
			logger.Error("authentication failed", "error", err)
			os.Exit(1)
		}
	}

	// Create tunnel manager and connect.
	tm := client.NewTunnelManager(c, logger)
	tm.OnStateChange(func(state client.TunnelState) {
		switch state {
		case client.TunnelConnected:
			fmt.Println("✓ VPN connected")
		case client.TunnelMFARequired:
			fmt.Println("⚠ MFA re-authentication required, please re-login")
			if err := doLogin(ctx, c); err != nil {
				logger.Error("re-authentication failed", "error", err)
			}
		case client.TunnelDisconnected:
			fmt.Println("✗ VPN disconnected")
		}
	})

	networkID := ""
	if len(os.Args) > 2 {
		networkID = os.Args[2]
	}

	if err := tm.Connect(ctx, networkID); err != nil {
		logger.Error("connection failed", "error", err)
		os.Exit(1)
	}

	// Run session loop (posture reporting, token refresh).
	go tm.RunSessionLoop(ctx, 5*time.Minute)

	fmt.Println("Press Ctrl+C to disconnect...")

	// Wait for signal.
	<-ctx.Done()

	fmt.Println("\nDisconnecting...")
	if err := tm.Disconnect(); err != nil {
		logger.Error("disconnect error", "error", err)
	}
}

func cmdGateway(c *client.Client, logger *slog.Logger) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: outpost-client gateway <tunnel-id>")
		os.Exit(1)
	}
	tunnelID := os.Args[2]

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Authenticate if needed.
	if err := c.LoadToken(); err != nil {
		if err := doLogin(ctx, c); err != nil {
			logger.Error("authentication failed", "error", err)
			os.Exit(1)
		}
	}

	gw := client.NewGatewayMode(c, tunnelID, logger)
	if err := gw.Start(ctx); err != nil {
		logger.Error("gateway start failed", "error", err)
		os.Exit(1)
	}

	fmt.Printf("S2S gateway running for tunnel %s. Press Ctrl+C to stop...\n", tunnelID)
	<-ctx.Done()

	fmt.Println("\nStopping gateway...")
	if err := gw.Stop(); err != nil {
		logger.Error("gateway stop error", "error", err)
	}
}

func cmdDisconnect(c *client.Client, logger *slog.Logger) {
	tm := client.NewTunnelManager(c, logger)
	if err := tm.Disconnect(); err != nil {
		logger.Error("disconnect failed", "error", err)
		os.Exit(1)
	}
	fmt.Println("Disconnected")
}

func cmdLogin(c *client.Client, logger *slog.Logger) {
	ctx := context.Background()
	if err := doLogin(ctx, c); err != nil {
		logger.Error("login failed", "error", err)
		os.Exit(1)
	}
	fmt.Println("Login successful. Token saved.")
}

func cmdStatus(_ *client.Client, _ *slog.Logger) {
	posture := client.CollectPosture()
	fmt.Printf("OS:         %s %s\n", posture.OSType, posture.OSVersion)
	fmt.Printf("Hostname:   %s\n", posture.Hostname)
	fmt.Println()

	// Try common Outpost WireGuard interface names.
	ifaceNames := []string{"outpost0", "wg0"}
	if v := os.Getenv("OUTPOST_WG_INTERFACE"); v != "" {
		ifaceNames = []string{v}
	}

	found := false
	for _, ifaceName := range ifaceNames {
		info, err := getWGInterfaceStatus(ifaceName)
		if err != nil {
			continue
		}
		found = true
		fmt.Printf("Interface:  %s\n", ifaceName)
		fmt.Printf("Status:     connected\n")
		if info.listenPort != "" {
			fmt.Printf("Listen Port: %s\n", info.listenPort)
		}
		if info.publicKey != "" {
			fmt.Printf("Public Key: %s\n", info.publicKey)
		}
		fmt.Println()

		if len(info.peers) == 0 {
			fmt.Println("Peers:      (none)")
		} else {
			fmt.Printf("Peers:      %d\n", len(info.peers))
			fmt.Println()
			for i, p := range info.peers {
				fmt.Printf("  Peer #%d\n", i+1)
				fmt.Printf("    Public Key:     %s\n", p.publicKey)
				if p.endpoint != "" {
					fmt.Printf("    Endpoint:       %s\n", p.endpoint)
				}
				if p.allowedIPs != "" {
					fmt.Printf("    Allowed IPs:    %s\n", p.allowedIPs)
				}
				if p.lastHandshake != "" && p.lastHandshake != "0" {
					fmt.Printf("    Last Handshake: %s\n", p.lastHandshake)
				} else {
					fmt.Printf("    Last Handshake: (never)\n")
				}
				fmt.Printf("    Transfer:       %s received, %s sent\n", p.rxBytes, p.txBytes)
			}
		}
		break // Show only the first active interface.
	}

	if !found {
		fmt.Println("Tunnel:     Not connected. Run 'outpost-client connect' to establish VPN.")
	}
}

type wgInterfaceInfo struct {
	publicKey  string
	listenPort string
	peers      []wgPeerInfo
}

type wgPeerInfo struct {
	publicKey     string
	endpoint      string
	allowedIPs    string
	lastHandshake string
	rxBytes       string
	txBytes       string
}

// getWGInterfaceStatus runs "wg show <iface> dump" and parses the output.
// Returns an error if the interface does not exist or wg is not available.
func getWGInterfaceStatus(ifaceName string) (*wgInterfaceInfo, error) {
	cmd := exec.Command("wg", "show", ifaceName, "dump")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("wg show failed: %w", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("no output from wg show")
	}

	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty wg show output")
	}

	info := &wgInterfaceInfo{}

	// First line: interface info
	// Format: private-key  public-key  listen-port  fwmark
	ifaceFields := strings.Split(lines[0], "\t")
	if len(ifaceFields) >= 2 {
		info.publicKey = ifaceFields[1]
	}
	if len(ifaceFields) >= 3 {
		info.listenPort = ifaceFields[2]
	}

	// Remaining lines: peers
	// Format: public-key  preshared-key  endpoint  allowed-ips  latest-handshake  transfer-rx  transfer-tx  persistent-keepalive
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}

		peer := wgPeerInfo{
			publicKey:  fields[0],
			endpoint:   fields[2],
			allowedIPs: fields[3],
		}

		// Parse last handshake unix timestamp into human-readable form.
		var handshakeUnix int64
		fmt.Sscanf(fields[4], "%d", &handshakeUnix)
		if handshakeUnix > 0 {
			t := time.Unix(handshakeUnix, 0)
			ago := time.Since(t).Truncate(time.Second)
			peer.lastHandshake = fmt.Sprintf("%s ago", ago)
		} else {
			peer.lastHandshake = ""
		}

		// Format transfer bytes in human-readable form.
		var rx, tx int64
		fmt.Sscanf(fields[5], "%d", &rx)
		fmt.Sscanf(fields[6], "%d", &tx)
		peer.rxBytes = formatBytes(rx)
		peer.txBytes = formatBytes(tx)

		info.peers = append(info.peers, peer)
	}

	return info, nil
}

// formatBytes converts a byte count into a human-readable string.
func formatBytes(b int64) string {
	const (
		kB = 1024
		mB = 1024 * kB
		gB = 1024 * mB
	)
	switch {
	case b >= gB:
		return fmt.Sprintf("%.2f GiB", float64(b)/float64(gB))
	case b >= mB:
		return fmt.Sprintf("%.2f MiB", float64(b)/float64(mB))
	case b >= kB:
		return fmt.Sprintf("%.2f KiB", float64(b)/float64(kB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func cmdPosture() {
	posture := client.CollectPosture()
	fmt.Printf("OS:               %s %s\n", posture.OSType, posture.OSVersion)
	fmt.Printf("Hostname:         %s\n", posture.Hostname)
	fmt.Printf("Disk Encrypted:   %s\n", boolStatus(posture.DiskEncrypted))
	fmt.Printf("Firewall:         %s\n", boolStatus(posture.FirewallEnabled))
	fmt.Printf("Screen Lock:      %s\n", boolStatus(posture.ScreenLockEnabled))
	fmt.Printf("Antivirus:        %s\n", boolStatus(posture.AntivirusActive))
}

func boolStatus(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func doLogin(ctx context.Context, c *client.Client) error {
	username := os.Getenv("OUTPOST_USER")
	password := os.Getenv("OUTPOST_PASS")

	if username == "" {
		fmt.Print("Username: ")
		username = readLine()
	}
	if password == "" {
		fmt.Print("Password: ")
		password = readLine()
	}

	resp, err := c.Login(ctx, username, password)
	if err != nil {
		return err
	}

	if resp.MFARequired {
		fmt.Println("MFA verification required.")
		fmt.Print("Enter MFA code: ")
		code := readLine()

		method := "totp"
		if len(code) == 8 {
			method = "backup"
		}

		if err := c.VerifyMFA(ctx, resp.MFAToken, code, method); err != nil {
			return fmt.Errorf("MFA verification failed: %w", err)
		}
		fmt.Println("MFA verified.")
	}

	return nil
}

func readLine() string {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}
