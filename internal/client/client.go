package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Client is the Outpost VPN client that handles authentication (including MFA),
// WireGuard tunnel management, and device posture reporting.
type Client struct {
	serverURL  string
	httpClient *http.Client
	token      string
	deviceID   string
	configDir  string
}

// NewClient creates a new Outpost VPN client.
func NewClient(serverURL string) *Client {
	return &Client{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		configDir: defaultConfigDir(),
	}
}

// --- Authentication Flow ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	MFARequired bool   `json:"mfa_required"`
	MFAToken    string `json:"mfa_token"`
}

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
	Method   string `json:"method"` // totp, email, backup
}

type mfaVerifyResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// Login authenticates the user and returns whether MFA is required.
func (c *Client) Login(ctx context.Context, username, password string) (*loginResponse, error) {
	body, err := json.Marshal(loginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth/login", body, false)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}

	if !result.MFARequired {
		c.token = result.Token
		if err := c.saveToken(result.Token); err != nil {
			return &result, fmt.Errorf("save token: %w", err)
		}
	}

	return &result, nil
}

// VerifyMFA completes the MFA challenge and obtains a full session token.
func (c *Client) VerifyMFA(ctx context.Context, mfaToken, code, method string) error {
	body, err := json.Marshal(mfaVerifyRequest{
		MFAToken: mfaToken,
		Code:     code,
		Method:   method,
	})
	if err != nil {
		return fmt.Errorf("marshal mfa request: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth/mfa/verify", body, false)
	if err != nil {
		return fmt.Errorf("verify mfa: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	var result mfaVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode mfa response: %w", err)
	}

	c.token = result.Token
	return c.saveToken(result.Token)
}

// --- Device Enrollment ---

type enrollmentRequest struct {
	PublicKey string         `json:"public_key"`
	Name      string         `json:"name"`
	Posture   *DevicePosture `json:"posture,omitempty"`
}

type enrollmentResponse struct {
	DeviceID   string          `json:"device_id"`
	AllowedIPs []string        `json:"allowed_ips"`
	DNS        []string        `json:"dns"`
	Endpoint   string          `json:"endpoint"`
	ServerKey  string          `json:"server_public_key"`
	Address    string          `json:"address"`
	Networks   []NetworkConfig `json:"networks"`
}

// NetworkConfig represents a VPN network available to the client.
type NetworkConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Address    string   `json:"address"`
	AllowedIPs []string `json:"allowed_ips"`
	DNS        []string `json:"dns"`
	Endpoint   string   `json:"endpoint"`
	ServerKey  string   `json:"server_public_key"`
}

// Enroll registers this device with the Outpost server.
func (c *Client) Enroll(ctx context.Context, publicKey, deviceName string) (*enrollmentResponse, error) {
	posture := CollectPosture()

	body, err := json.Marshal(enrollmentRequest{
		PublicKey: publicKey,
		Name:      deviceName,
		Posture:   posture,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal enrollment: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/devices/enroll", body, true)
	if err != nil {
		return nil, fmt.Errorf("enroll: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.readError(resp)
	}

	var result enrollmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode enrollment response: %w", err)
	}

	c.deviceID = result.DeviceID
	return &result, nil
}

// --- Posture Reporting ---

// DevicePosture represents the security state of this device.
type DevicePosture struct {
	OSType            string `json:"os_type"`
	OSVersion         string `json:"os_version"`
	Hostname          string `json:"hostname"`
	DiskEncrypted     bool   `json:"disk_encrypted"`
	ScreenLockEnabled bool   `json:"screen_lock_enabled"`
	AntivirusActive   bool   `json:"antivirus_active"`
	FirewallEnabled   bool   `json:"firewall_enabled"`
}

// CollectPosture gathers the current device security posture.
func CollectPosture() *DevicePosture {
	hostname, _ := os.Hostname()
	posture := &DevicePosture{
		OSType:   runtime.GOOS,
		Hostname: hostname,
	}
	collectPlatformPosture(posture)
	return posture
}

// ReportPosture sends the current device posture to the server.
func (c *Client) ReportPosture(ctx context.Context) error {
	posture := CollectPosture()
	body, err := json.Marshal(posture)
	if err != nil {
		return fmt.Errorf("marshal posture: %w", err)
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/devices/"+c.deviceID+"/posture", body, true)
	if err != nil {
		return fmt.Errorf("report posture: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	return nil
}

// --- Session Management ---

// RefreshSession refreshes the current session before it expires.
func (c *Client) RefreshSession(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth/refresh", nil, true)
	if err != nil {
		return fmt.Errorf("refresh session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.readError(resp)
	}

	var result loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode refresh response: %w", err)
	}

	c.token = result.Token
	return c.saveToken(result.Token)
}

// Logout invalidates the current session.
func (c *Client) Logout(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/auth/logout", nil, true)
	if err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	defer resp.Body.Close()

	c.token = ""
	_ = os.Remove(c.tokenPath())
	return nil
}

// --- HTTP Helpers ---

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte, auth bool) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "outpost-client/1.0")

	if auth && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	return c.httpClient.Do(req)
}

type apiError struct {
	Error string `json:"error"`
}

func (c *Client) readError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)
	var apiErr apiError
	if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Error != "" {
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, apiErr.Error)
	}
	return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(data))
}

// --- Token Persistence ---

func (c *Client) saveToken(token string) error {
	if err := os.MkdirAll(c.configDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(c.tokenPath(), []byte(token), 0600)
}

// LoadToken loads a previously saved authentication token.
func (c *Client) LoadToken() error {
	data, err := os.ReadFile(c.tokenPath())
	if err != nil {
		return err
	}
	c.token = string(data)
	return nil
}

func (c *Client) tokenPath() string {
	return filepath.Join(c.configDir, "token")
}

func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Fallback to /tmp to avoid writing to filesystem root.
		return filepath.Join(os.TempDir(), ".outpost")
	}
	return filepath.Join(home, ".outpost")
}
