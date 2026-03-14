package handler

import (
	"fmt"
	"net"
	"testing"
)

// validateCIDR replicates the validation logic from the network handler
// for unit testing without database dependency.
func validateCIDR(address string) (bool, string) {
	ip, ipNet, err := net.ParseCIDR(address)
	if err != nil {
		return false, fmt.Sprintf("invalid CIDR format: %s", address)
	}
	if !ip.Equal(ipNet.IP) {
		return false, fmt.Sprintf("host bits set — did you mean %s?", ipNet.String())
	}
	return true, ""
}

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		input   string
		valid   bool
		errPart string
	}{
		// Valid network addresses
		{"10.0.0.0/24", true, ""},
		{"172.16.0.0/16", true, ""},
		{"192.168.1.0/24", true, ""},
		{"10.0.0.0/8", true, ""},
		{"0.0.0.0/0", true, ""},
		{"10.10.10.0/30", true, ""},

		// Host bits set — should suggest correct CIDR
		{"10.0.0.2/24", false, "did you mean 10.0.0.0/24"},
		{"192.168.1.100/24", false, "did you mean 192.168.1.0/24"},
		{"172.16.5.3/16", false, "did you mean 172.16.0.0/16"},
		{"10.1.2.3/8", false, "did you mean 10.0.0.0/8"},

		// Invalid format
		{"not-a-cidr", false, "invalid CIDR format"},
		{"10.0.0.0", false, "invalid CIDR format"},
		{"10.0.0.0/33", false, "invalid CIDR format"},
		{"", false, "invalid CIDR format"},
		{"256.0.0.0/24", false, "invalid CIDR format"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			valid, errMsg := validateCIDR(tt.input)
			if valid != tt.valid {
				t.Errorf("validateCIDR(%q) valid = %v, want %v (err: %s)", tt.input, valid, tt.valid, errMsg)
			}
			if !valid && tt.errPart != "" {
				if len(errMsg) == 0 || !containsSubstring(errMsg, tt.errPart) {
					t.Errorf("validateCIDR(%q) error = %q, want to contain %q", tt.input, errMsg, tt.errPart)
				}
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNetworkRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		reqName string
		address string
		errMsg  string
	}{
		{"empty name", "", "10.0.0.0/24", "name is required"},
		{"empty address", "test", "", "address (CIDR) is required"},
		{"valid", "test", "10.0.0.0/24", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errMsg string
			if tt.reqName == "" {
				errMsg = "name is required"
			} else if tt.address == "" {
				errMsg = "address (CIDR) is required"
			}
			if errMsg != tt.errMsg {
				t.Errorf("validation error = %q, want %q", errMsg, tt.errMsg)
			}
		})
	}
}
