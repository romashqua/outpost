package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/romashqua/outpost/pkg/version"
)

var (
	apiURL = envOr("OUTPOST_API_URL", "http://localhost:8080")
	token  = os.Getenv("OUTPOST_TOKEN")
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "version":
		fmt.Printf("outpostctl %s\n", version.Version)
	case "login":
		err = cmdLogin()
	case "users":
		err = cmdUsers()
	case "networks":
		err = cmdNetworks()
	case "devices":
		err = cmdDevices()
	case "gateways":
		err = cmdGateways()
	case "audit":
		err = cmdAudit()
	case "compliance":
		err = cmdCompliance()
	case "status":
		err = cmdStatus()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`outpostctl - Outpost VPN management CLI

Usage:
  outpostctl <command> [subcommand] [flags]

Commands:
  version       Print version information
  login         Authenticate and store token
  status        Show cluster health and readiness
  users         Manage users (list, get, create, delete)
  networks      Manage VPN networks (list, get, create, delete)
  devices       Manage devices (list, get, approve, revoke, delete)
  gateways      Manage gateways (list, get, create, delete)
  audit         View audit log
  compliance    Run compliance checks

Environment:
  OUTPOST_API_URL   API base URL (default: http://localhost:8080)
  OUTPOST_TOKEN     JWT authentication token

Examples:
  outpostctl login -u admin -p secret
  outpostctl users list
  outpostctl networks create -n "Office VPN" -a 10.10.0.0/24
  outpostctl compliance report
  outpostctl audit --action user.created --limit 20
`)
}

// ── Login ────────────────────────────────────────────────────────────

func cmdLogin() error {
	args := os.Args[2:]
	var username, password string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-u", "--username":
			i++
			if i < len(args) {
				username = args[i]
			}
		case "-p", "--password":
			i++
			if i < len(args) {
				password = args[i]
			}
		}
	}
	if username == "" || password == "" {
		return fmt.Errorf("usage: outpostctl login -u <username> -p <password>")
	}

	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := http.Post(apiURL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %v", result["error"])
	}

	fmt.Printf("Authenticated successfully.\n")
	fmt.Printf("Export token:\n  export OUTPOST_TOKEN=%s\n", result["token"])
	return nil
}

// ── Status ───────────────────────────────────────────────────────────

func cmdStatus() error {
	healthResp, err := http.Get(apiURL + "/healthz")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer healthResp.Body.Close()

	readyResp, err := http.Get(apiURL + "/readyz")
	if err != nil {
		return fmt.Errorf("readiness check failed: %w", err)
	}
	defer readyResp.Body.Close()

	fmt.Printf("API:       %s\n", apiURL)
	fmt.Printf("Health:    %s\n", statusIcon(healthResp.StatusCode == 200))
	fmt.Printf("Ready:     %s\n", statusIcon(readyResp.StatusCode == 200))
	return nil
}

// ── Users ────────────────────────────────────────────────────────────

func cmdUsers() error {
	sub := subcommand()
	switch sub {
	case "list", "":
		return listResource("/api/v1/users", "data", []string{"ID", "USERNAME", "EMAIL", "ACTIVE", "ADMIN"},
			func(item map[string]any) []string {
				return []string{
					strVal(item["id"]),
					strVal(item["username"]),
					strVal(item["email"]),
					boolIcon(item["is_active"]),
					boolIcon(item["is_admin"]),
				}
			})
	case "get":
		return getResource("/api/v1/users/" + requireArg(3, "user ID"))
	case "delete":
		return deleteResource("/api/v1/users/" + requireArg(3, "user ID"))
	case "create":
		return createUser()
	default:
		return fmt.Errorf("unknown subcommand: users %s", sub)
	}
}

func createUser() error {
	args := os.Args[3:]
	body := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-u", "--username":
			i++
			body["username"] = args[i]
		case "-e", "--email":
			i++
			body["email"] = args[i]
		case "-p", "--password":
			i++
			body["password"] = args[i]
		case "--admin":
			body["is_admin"] = true
		}
	}
	return postResource("/api/v1/users", body)
}

// ── Networks ─────────────────────────────────────────────────────────

func cmdNetworks() error {
	sub := subcommand()
	switch sub {
	case "list", "":
		return listResource("/api/v1/networks", "", []string{"ID", "NAME", "ADDRESS", "PORT", "ACTIVE"},
			func(item map[string]any) []string {
				return []string{
					strVal(item["id"]),
					strVal(item["name"]),
					strVal(item["address"]),
					strVal(item["port"]),
					boolIcon(item["is_active"]),
				}
			})
	case "get":
		return getResource("/api/v1/networks/" + requireArg(3, "network ID"))
	case "delete":
		return deleteResource("/api/v1/networks/" + requireArg(3, "network ID"))
	case "create":
		return createNetwork()
	default:
		return fmt.Errorf("unknown subcommand: networks %s", sub)
	}
}

func createNetwork() error {
	args := os.Args[3:]
	body := map[string]any{"port": 51820, "keepalive": 25}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--name":
			i++
			body["name"] = args[i]
		case "-a", "--address":
			i++
			body["address"] = args[i]
		}
	}
	return postResource("/api/v1/networks", body)
}

// ── Devices ──────────────────────────────────────────────────────────

func cmdDevices() error {
	sub := subcommand()
	switch sub {
	case "list", "":
		return listResource("/api/v1/devices", "", []string{"ID", "USER", "NAME", "IP", "APPROVED"},
			func(item map[string]any) []string {
				return []string{
					strVal(item["id"]),
					shortUUID(strVal(item["user_id"])),
					strVal(item["name"]),
					strVal(item["assigned_ip"]),
					boolIcon(item["is_approved"]),
				}
			})
	case "get":
		return getResource("/api/v1/devices/" + requireArg(3, "device ID"))
	case "approve":
		return doAction("/api/v1/devices/"+requireArg(3, "device ID")+"/approve", "POST")
	case "revoke":
		return doAction("/api/v1/devices/"+requireArg(3, "device ID")+"/revoke", "POST")
	case "delete":
		return deleteResource("/api/v1/devices/" + requireArg(3, "device ID"))
	default:
		return fmt.Errorf("unknown subcommand: devices %s", sub)
	}
}

// ── Gateways ─────────────────────────────────────────────────────────

func cmdGateways() error {
	sub := subcommand()
	switch sub {
	case "list", "":
		return listResource("/api/v1/gateways", "", []string{"ID", "NAME", "NETWORK", "ENDPOINT", "ACTIVE", "LAST SEEN"},
			func(item map[string]any) []string {
				return []string{
					strVal(item["id"]),
					strVal(item["name"]),
					shortUUID(strVal(item["network_id"])),
					strVal(item["endpoint"]),
					boolIcon(item["is_active"]),
					timeAgo(strVal(item["last_seen"])),
				}
			})
	case "get":
		return getResource("/api/v1/gateways/" + requireArg(3, "gateway ID"))
	case "delete":
		return deleteResource("/api/v1/gateways/" + requireArg(3, "gateway ID"))
	default:
		return fmt.Errorf("unknown subcommand: gateways %s", sub)
	}
}

// ── Audit ────────────────────────────────────────────────────────────

func cmdAudit() error {
	params := "?per_page=20"
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--action":
			i++
			params += "&action=" + args[i]
		case "--user":
			i++
			params += "&user_id=" + args[i]
		case "--limit":
			i++
			params += "&per_page=" + args[i]
		}
	}
	return listResource("/api/v1/audit"+params, "data", []string{"ID", "TIMESTAMP", "USER", "ACTION", "RESOURCE", "IP"},
		func(item map[string]any) []string {
			return []string{
				strVal(item["id"]),
				timeShort(strVal(item["timestamp"])),
				shortUUID(strVal(item["user_id"])),
				strVal(item["action"]),
				strVal(item["resource"]),
				strVal(item["ip_address"]),
			}
		})
}

// ── Compliance ───────────────────────────────────────────────────────

func cmdCompliance() error {
	sub := subcommand()
	switch sub {
	case "report", "":
		resp, err := apiGet("/api/v1/compliance/report")
		if err != nil {
			return err
		}
		var report map[string]any
		if err := json.Unmarshal(resp, &report); err != nil {
			return err
		}
		fmt.Printf("Compliance Report\n")
		fmt.Printf("─────────────────────────────────────\n")
		fmt.Printf("Overall Score:     %v/100\n", report["overall_score"])
		fmt.Printf("MFA Adoption:      %.0f%%\n", toFloat(report["mfa_adoption"])*100)
		fmt.Printf("Encryption Rate:   %.0f%%\n", toFloat(report["encryption_rate"])*100)
		fmt.Printf("Posture Rate:      %.0f%%\n", toFloat(report["posture_rate"])*100)
		fmt.Printf("Audit Log:         %s\n", boolIcon(report["audit_log_enabled"]))
		fmt.Printf("Password Policy:   %s\n", boolIcon(report["password_policy"]))
		fmt.Printf("Session Timeout:   %s\n", boolIcon(report["session_timeout"]))

		if checks, ok := report["checks"].([]any); ok {
			fmt.Printf("\nChecks (%d):\n", len(checks))
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "STATUS\tCATEGORY\tNAME")
			for _, c := range checks {
				if m, ok := c.(map[string]any); ok {
					icon := "FAIL"
					switch strVal(m["status"]) {
					case "pass":
						icon = "PASS"
					case "warning":
						icon = "WARN"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\n", icon, strVal(m["category"]), strVal(m["name"]))
				}
			}
			w.Flush()
		}
		return nil
	case "soc2":
		return getResource("/api/v1/compliance/soc2")
	case "iso27001":
		return getResource("/api/v1/compliance/iso27001")
	case "gdpr":
		return getResource("/api/v1/compliance/gdpr")
	default:
		return fmt.Errorf("unknown subcommand: compliance %s", sub)
	}
}

// ── HTTP helpers ─────────────────────────────────────────────────────

func apiRequest(method, path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(method, apiURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return data, resp.StatusCode, nil
}

func apiGet(path string) ([]byte, error) {
	data, status, err := apiRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", status, string(data))
	}
	return data, nil
}

func listResource(path, dataKey string, headers []string, rowFn func(map[string]any) []string) error {
	data, err := apiGet(path)
	if err != nil {
		return err
	}

	var items []map[string]any
	if dataKey != "" {
		var wrapped map[string]any
		if err := json.Unmarshal(data, &wrapped); err != nil {
			return err
		}
		if arr, ok := wrapped[dataKey].([]any); ok {
			for _, a := range arr {
				if m, ok := a.(map[string]any); ok {
					items = append(items, m)
				}
			}
		}
	} else {
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, item := range items {
		fmt.Fprintln(w, strings.Join(rowFn(item), "\t"))
	}
	w.Flush()
	fmt.Printf("\nTotal: %d\n", len(items))
	return nil
}

func getResource(path string) error {
	data, err := apiGet(path)
	if err != nil {
		return err
	}
	var pretty bytes.Buffer
	json.Indent(&pretty, data, "", "  ")
	fmt.Println(pretty.String())
	return nil
}

func deleteResource(path string) error {
	_, status, err := apiRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete failed with HTTP %d", status)
	}
	fmt.Println("Deleted.")
	return nil
}

func postResource(path string, body map[string]any) error {
	jsonBody, _ := json.Marshal(body)
	data, status, err := apiRequest("POST", path, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("HTTP %d: %s", status, string(data))
	}
	var pretty bytes.Buffer
	json.Indent(&pretty, data, "", "  ")
	fmt.Println(pretty.String())
	return nil
}

func doAction(path, method string) error {
	data, status, err := apiRequest(method, path, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("HTTP %d: %s", status, string(data))
	}
	fmt.Println("Done.")
	return nil
}

// ── Formatting helpers ───────────────────────────────────────────────

func subcommand() string {
	if len(os.Args) > 2 {
		return os.Args[2]
	}
	return ""
}

func requireArg(idx int, name string) string {
	if len(os.Args) <= idx {
		fmt.Fprintf(os.Stderr, "missing required argument: %s\n", name)
		os.Exit(1)
	}
	return os.Args[idx]
}

func strVal(v any) string {
	if v == nil {
		return "-"
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%.2f", t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func shortUUID(s string) string {
	if len(s) >= 8 {
		return s[:8]
	}
	return s
}

func boolIcon(v any) string {
	switch b := v.(type) {
	case bool:
		if b {
			return "YES"
		}
		return "NO"
	default:
		return "-"
	}
}

func statusIcon(ok bool) string {
	if ok {
		return "OK"
	}
	return "FAIL"
}

func toFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func timeAgo(s string) string {
	if s == "" || s == "-" || s == "<nil>" {
		return "never"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func timeShort(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("01-02 15:04")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
