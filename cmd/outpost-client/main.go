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
	"github.com/romashqua/outpost/pkg/cli"
	"github.com/romashqua/outpost/pkg/version"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	serverURL := os.Getenv("OUTPOST_SERVER")
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	c := client.NewClient(serverURL)

	if len(os.Args) < 2 {
		printBanner(serverURL)
		runInteractive(c, logger, serverURL)
		return
	}

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
		fmt.Printf("outpost-client %s (%s) built %s\n", version.Version, version.GitCommit, version.BuildDate)
	case "completion":
		printCompletion()
	case "help", "--help", "-h":
		printBanner(serverURL)
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "%s\n", cli.ErrorMsg("unknown command: "+os.Args[1]))
		printUsage()
		os.Exit(1)
	}
}

func runInteractive(c *client.Client, logger *slog.Logger, serverURL string) {
	if !cli.IsTerminal() {
		printUsage()
		return
	}

	repl := cli.NewREPL("outpost-client")
	repl.Register(cli.Command{Name: "connect", Description: "Connect to VPN", Run: func(args []string) error {
		os.Args = append([]string{"outpost-client", "connect"}, args...)
		cmdConnect(c, logger)
		return nil
	}})
	repl.Register(cli.Command{Name: "disconnect", Description: "Disconnect from VPN", Run: func(_ []string) error {
		cmdDisconnect(c, logger)
		return nil
	}})
	repl.Register(cli.Command{Name: "login", Description: "Authenticate", Run: func(_ []string) error {
		cmdLogin(c, logger)
		return nil
	}})
	repl.Register(cli.Command{Name: "status", Description: "Show connection status", Run: func(_ []string) error {
		cmdStatus(c, logger)
		return nil
	}})
	repl.Register(cli.Command{Name: "posture", Description: "Show device security posture", Run: func(_ []string) error {
		cmdPosture()
		return nil
	}})
	repl.Register(cli.Command{Name: "version", Description: "Show version info", Run: func(_ []string) error {
		fmt.Printf("outpost-client %s (%s) built %s\n", version.Version, version.GitCommit, version.BuildDate)
		return nil
	}})

	// Single-key shortcuts.
	repl.Alias("q", "quit")
	repl.Alias("h", "help")
	repl.Alias("?", "help")
	repl.Alias("c", "connect")
	repl.Alias("d", "disconnect")
	repl.Alias("l", "login")
	repl.Alias("s", "status")
	repl.Alias("p", "posture")
	repl.Alias("v", "version")

	repl.Run()
}

func printBanner(serverURL string) {
	if !cli.IsTerminal() {
		return
	}

	extra := []string{
		fmt.Sprintf("%sServer:%s %s", cli.Muted, cli.Reset, serverURL),
	}

	fmt.Print(cli.Banner("outpost-client", version.Version, version.GitCommit, extra))
}

func printUsage() {
	fmt.Printf(`
%s%sUsage:%s  outpost-client <command> [options]

%s%sCommands:%s
  %sconnect%s      Connect to VPN (login + MFA + tunnel)
  %sdisconnect%s   Disconnect from VPN
  %sgateway%s      Run as S2S gateway (gateway <tunnel-id>)
  %slogin%s        Authenticate without connecting
  %sstatus%s       Show connection status
  %sposture%s      Show device security posture
  %sversion%s      Show version
  %scompletion%s   Generate shell completion (bash|zsh)

%s%sEnvironment:%s
  %sOUTPOST_SERVER%s   Server URL (default: http://localhost:8080)
  %sOUTPOST_USER%s     Username for non-interactive login
  %sOUTPOST_PASS%s     Password for non-interactive login
`,
		cli.Bold, cli.White, cli.Reset,
		cli.Bold, cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Accent, cli.Reset,
		cli.Bold, cli.Muted, cli.Reset,
		cli.Yellow, cli.Reset,
		cli.Yellow, cli.Reset,
		cli.Yellow, cli.Reset,
	)
}

var clientCommands = []cli.CommandDef{
	{Name: "connect", Description: "Connect to VPN"},
	{Name: "disconnect", Description: "Disconnect from VPN"},
	{Name: "gateway", Description: "Run as S2S gateway"},
	{Name: "login", Description: "Authenticate without connecting"},
	{Name: "status", Description: "Show connection status"},
	{Name: "posture", Description: "Show device security posture"},
	{Name: "version", Description: "Show version"},
	{Name: "completion", Description: "Generate shell completion"},
	{Name: "help", Description: "Show help"},
}

func printCompletion() {
	shell := "bash"
	if len(os.Args) > 2 {
		shell = os.Args[2]
	}
	switch shell {
	case "bash":
		cmdNames := make([]string, len(clientCommands))
		for i, c := range clientCommands {
			cmdNames[i] = c.Name
		}
		fmt.Print(cli.BashCompletion("outpost-client", cmdNames))
	case "zsh":
		fmt.Print(cli.ZshCompletion("outpost-client", clientCommands))
	default:
		fmt.Fprintf(os.Stderr, "%s\n", cli.ErrorMsg("unsupported shell: "+shell+". Use bash or zsh."))
		os.Exit(1)
	}
}

func cmdConnect(c *client.Client, logger *slog.Logger) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// If credentials are provided via env vars, always do a fresh login
	// (saved token may have been signed with a different JWT secret).
	if os.Getenv("OUTPOST_USER") != "" && os.Getenv("OUTPOST_PASS") != "" {
		if err := doLogin(ctx, c); err != nil {
			logger.Error("authentication failed", "error", err)
			os.Exit(1)
		}
	} else if err := c.LoadToken(); err != nil {
		// No env vars — try loading saved token, fall back to interactive login.
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
			fmt.Println(cli.SuccessMsg("VPN connected"))
		case client.TunnelMFARequired:
			fmt.Println(cli.WarnMsg("MFA re-authentication required, please re-login"))
			if err := doLogin(ctx, c); err != nil {
				logger.Error("re-authentication failed", "error", err)
			}
		case client.TunnelDisconnected:
			fmt.Printf("%s%s✗%s VPN disconnected\n", cli.Red, cli.Bold, cli.Reset)
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
