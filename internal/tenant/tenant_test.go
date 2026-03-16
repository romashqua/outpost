package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractSubdomain(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"acme.outpost.example.com", "acme"},
		{"acme.outpost.example.com:8080", "acme"},
		{"outpost.example.com", "outpost"},
		{"outpost.example.com:443", "outpost"},
		{"localhost", ""},
		{"localhost:8080", ""},
		{"a.b.c.d.e", "a"},
		{"tenant1.vpn.company.io", "tenant1"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := extractSubdomain(tt.host)
			if got != tt.want {
				t.Errorf("extractSubdomain(%q)=%q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

func TestWithTenant_FromContext(t *testing.T) {
	tenant := &Tenant{
		ID:   "t-1",
		Name: "Acme Corp",
		Slug: "acme",
		Plan: "pro",
	}

	ctx := WithTenant(context.Background(), tenant)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("expected tenant in context")
	}
	if got.ID != "t-1" {
		t.Errorf("ID=%q, want t-1", got.ID)
	}
	if got.Name != "Acme Corp" {
		t.Errorf("Name=%q, want Acme Corp", got.Name)
	}
}

func TestFromContext_Empty(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Error("expected no tenant in empty context")
	}
}

func TestTenant_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tenant := Tenant{
		ID:          "t-123",
		Name:        "Test Org",
		Slug:        "test-org",
		Plan:        "enterprise",
		MaxUsers:    10000,
		MaxDevices:  50000,
		MaxNetworks: 200,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := json.Marshal(tenant)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Tenant
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != tenant.ID {
		t.Errorf("ID=%q, want %q", decoded.ID, tenant.ID)
	}
	if decoded.Plan != "enterprise" {
		t.Errorf("Plan=%q, want enterprise", decoded.Plan)
	}
	if decoded.MaxUsers != 10000 {
		t.Errorf("MaxUsers=%d, want 10000", decoded.MaxUsers)
	}
	if decoded.IsActive != true {
		t.Error("IsActive should be true")
	}
}

func TestTenant_JSONKeys(t *testing.T) {
	tenant := Tenant{
		ID:          "id-1",
		Name:        "n",
		Slug:        "s",
		Plan:        "free",
		MaxUsers:    10,
		MaxDevices:  20,
		MaxNetworks: 2,
		IsActive:    false,
	}

	data, err := json.Marshal(tenant)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	expectedKeys := []string{"id", "name", "slug", "plan", "max_users", "max_devices", "max_networks", "is_active", "created_at", "updated_at"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusForbidden, "access denied")

	if w.Code != http.StatusForbidden {
		t.Errorf("status=%d, want %d", w.Code, http.StatusForbidden)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q, want application/json", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["error"] != "access denied" {
		t.Errorf("error=%q, want %q", result["error"], "access denied")
	}
}

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, map[string]string{"status": "ok"})

	if w.Code != http.StatusCreated {
		t.Errorf("status=%d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type=%q, want application/json", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status=%q, want ok", result["status"])
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

func TestParseBody_Valid(t *testing.T) {
	body := `{"name": "Acme", "slug": "acme"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst createTenantRequest
	if err := parseBody(req, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Name != "Acme" {
		t.Errorf("Name=%q, want Acme", dst.Name)
	}
	if dst.Slug != "acme" {
		t.Errorf("Slug=%q, want acme", dst.Slug)
	}
}

func TestParseBody_InvalidJSON(t *testing.T) {
	body := `{not valid json}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst createTenantRequest
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

	var dst createTenantRequest
	err := parseBody(req, &dst)
	if err == nil {
		t.Fatal("expected error for nil body")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error=%q, want it to contain 'empty'", err.Error())
	}
}

func TestParseBody_UnknownFields(t *testing.T) {
	body := `{"name": "Acme", "slug": "acme", "unknown_field": true}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var dst createTenantRequest
	err := parseBody(req, &dst)
	if err == nil {
		t.Fatal("expected error for unknown fields (DisallowUnknownFields)")
	}
}

func TestTenantStats_JSONSerialization(t *testing.T) {
	s := tenantStats{
		TenantID:     "t-1",
		UserCount:    50,
		DeviceCount:  120,
		NetworkCount: 5,
		GatewayCount: 3,
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded["tenant_id"] != "t-1" {
		t.Errorf("tenant_id=%v, want t-1", decoded["tenant_id"])
	}
	if int(decoded["user_count"].(float64)) != 50 {
		t.Errorf("user_count=%v, want 50", decoded["user_count"])
	}
}

func TestUpdateTenantRequest_PartialFields(t *testing.T) {
	body := `{"name": "New Name"}`
	var req updateTenantRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if req.Name == nil || *req.Name != "New Name" {
		t.Errorf("Name=%v, want pointer to 'New Name'", req.Name)
	}
	if req.Slug != nil {
		t.Error("Slug should be nil for partial update")
	}
	if req.Plan != nil {
		t.Error("Plan should be nil for partial update")
	}
	if req.IsActive != nil {
		t.Error("IsActive should be nil for partial update")
	}
}
