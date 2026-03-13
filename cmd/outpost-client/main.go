package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
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
			fmt.Println("⚠ MFA re-authentication required")
			fmt.Print("Enter MFA code: ")
			code := readLine()
			if err := c.VerifyMFA(ctx, "", code, "totp"); err != nil {
				logger.Error("MFA verification failed", "error", err)
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
	// TODO: show tunnel status by checking wg interface.
	fmt.Println("Tunnel:     run 'outpost-client connect' to establish VPN")
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
