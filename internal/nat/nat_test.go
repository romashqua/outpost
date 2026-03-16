package nat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNATTypeConstants(t *testing.T) {
	types := map[NATType]string{
		NATTypeFullCone:       "full_cone",
		NATTypeRestrictedCone: "restricted_cone",
		NATTypePortRestricted: "port_restricted",
		NATTypeSymmetric:      "symmetric",
		NATTypeOpen:           "open",
		NATTypeUnknown:        "unknown",
	}

	for natType, expected := range types {
		if string(natType) != expected {
			t.Errorf("NATType %v = %q, want %q", natType, string(natType), expected)
		}
	}
}

func TestDiscoveryResult_JSONSerialization(t *testing.T) {
	dr := DiscoveryResult{
		NATType:      NATTypeSymmetric,
		ExternalIP:   "203.0.113.1",
		ExternalPort: 12345,
	}

	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded DiscoveryResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.NATType != NATTypeSymmetric {
		t.Errorf("NATType=%q, want %q", decoded.NATType, NATTypeSymmetric)
	}
	if decoded.ExternalIP != "203.0.113.1" {
		t.Errorf("ExternalIP=%q, want 203.0.113.1", decoded.ExternalIP)
	}
	if decoded.ExternalPort != 12345 {
		t.Errorf("ExternalPort=%d, want 12345", decoded.ExternalPort)
	}
}

func TestDiscoveryResult_JSONKeys(t *testing.T) {
	dr := DiscoveryResult{
		NATType:      NATTypeOpen,
		ExternalIP:   "1.2.3.4",
		ExternalPort: 100,
	}

	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	for _, key := range []string{"nat_type", "external_ip", "external_port"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestValidateRealm(t *testing.T) {
	tests := []struct {
		realm string
		valid bool
	}{
		{"outpost.vpn", true},
		{"my-realm", true},
		{"realm_1", true},
		{"UPPER.lower", true},
		{"a", true},
		{"", false},
		{"has space", false},
		{"has/slash", false},
		{"has@at", false},
		{strings.Repeat("a", 255), true},
		{strings.Repeat("a", 256), false},
	}

	for _, tt := range tests {
		t.Run(tt.realm, func(t *testing.T) {
			got := ValidateRealm(tt.realm)
			if got != tt.valid {
				t.Errorf("ValidateRealm(%q)=%v, want %v", tt.realm, got, tt.valid)
			}
		})
	}
}

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status=%d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q, want application/json", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("key=%q, want value", result["key"])
	}
}

func TestRespondJSON_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("status=%d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want %d", w.Code, http.StatusBadRequest)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["error"] != "bad input" {
		t.Errorf("error=%q, want 'bad input'", result["error"])
	}
	if result["message"] != "bad input" {
		t.Errorf("message=%q, want 'bad input'", result["message"])
	}
}

func TestParseBody_Valid(t *testing.T) {
	body := `{"device_id":"abc-123","stun_server_1":"stun.example.com:3478"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst natCheckRequest
	if err := parseBody(req, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.DeviceID != "abc-123" {
		t.Errorf("DeviceID=%q, want abc-123", dst.DeviceID)
	}
	if dst.STUNServer1 != "stun.example.com:3478" {
		t.Errorf("STUNServer1=%q, want stun.example.com:3478", dst.STUNServer1)
	}
}

func TestParseBody_InvalidJSON(t *testing.T) {
	body := `{broken`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst natCheckRequest
	err := parseBody(req, &dst)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error=%q, want it to contain 'invalid JSON'", err.Error())
	}
}

func TestParseBody_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil

	var dst natCheckRequest
	err := parseBody(req, &dst)
	if err == nil {
		t.Fatal("expected error for nil body")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error=%q, want it to contain 'empty'", err.Error())
	}
}

func TestParseBody_UnknownFields(t *testing.T) {
	body := `{"device_id":"x","bogus_field":true}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst natCheckRequest
	err := parseBody(req, &dst)
	if err == nil {
		t.Fatal("expected error for unknown fields")
	}
}

func TestRelayServerResponse_JSONKeys(t *testing.T) {
	rs := relayServerResponse{
		ID:       "r-1",
		Name:     "relay-eu",
		Address:  "turn.example.com:3478",
		Region:   "eu-west",
		Protocol: "turn",
		IsActive: true,
	}

	data, err := json.Marshal(rs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	expectedKeys := []string{"id", "name", "address", "region", "protocol", "is_active", "created_at", "updated_at"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestNATStatusResponse_JSONKeys(t *testing.T) {
	ns := natStatusResponse{
		DeviceID:     "d-1",
		NATType:      "symmetric",
		ExternalIP:   "1.2.3.4",
		ExternalPort: 5555,
	}

	data, err := json.Marshal(ns)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	for _, key := range []string{"device_id", "nat_type", "external_ip", "external_port", "last_checked", "created_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// relay_server_id should be omitted when nil
	if _, ok := raw["relay_server_id"]; ok {
		t.Error("relay_server_id should be omitted when nil")
	}
}

func TestNATCheckResponse_JSONSerialization(t *testing.T) {
	resp := natCheckResponse{
		NATType:      "full_cone",
		ExternalIP:   "198.51.100.1",
		ExternalPort: 9999,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded natCheckResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.NATType != "full_cone" {
		t.Errorf("NATType=%q, want full_cone", decoded.NATType)
	}
	if decoded.ExternalPort != 9999 {
		t.Errorf("ExternalPort=%d, want 9999", decoded.ExternalPort)
	}
}

func TestNewSTUNServer(t *testing.T) {
	s := NewSTUNServer(":3478", nil)
	if s == nil {
		t.Fatal("NewSTUNServer returned nil")
	}
	if s.listenAddr != ":3478" {
		t.Errorf("listenAddr=%q, want :3478", s.listenAddr)
	}
}

func TestSTUNServer_Close_NoConn(t *testing.T) {
	s := NewSTUNServer(":3478", nil)
	// Close without Start should not panic or error.
	if err := s.Close(); err != nil {
		t.Errorf("Close with no conn: unexpected error: %v", err)
	}
}

func TestNewTURNServer(t *testing.T) {
	s := NewTURNServer(":3479", "outpost.vpn", "1.2.3.4", nil)
	if s == nil {
		t.Fatal("NewTURNServer returned nil")
	}
	if s.listenAddr != ":3479" {
		t.Errorf("listenAddr=%q, want :3479", s.listenAddr)
	}
	if s.realm != "outpost.vpn" {
		t.Errorf("realm=%q, want outpost.vpn", s.realm)
	}
	if s.externalIP != "1.2.3.4" {
		t.Errorf("externalIP=%q, want 1.2.3.4", s.externalIP)
	}
}

func TestTURNServer_Close_NoServer(t *testing.T) {
	s := NewTURNServer(":3479", "realm", "1.2.3.4", nil)
	// Close without Start should not panic or error.
	if err := s.Close(); err != nil {
		t.Errorf("Close with no server: unexpected error: %v", err)
	}
}
