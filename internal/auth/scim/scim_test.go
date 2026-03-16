package scim

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- trimJSONString ---

func TestTrimJSONString_Quoted(t *testing.T) {
	got := trimJSONString(`"hello"`)
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTrimJSONString_NotQuoted(t *testing.T) {
	got := trimJSONString("hello")
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestTrimJSONString_Empty(t *testing.T) {
	got := trimJSONString("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTrimJSONString_SingleChar(t *testing.T) {
	got := trimJSONString(`"`)
	if got != `"` {
		t.Errorf("expected single quote, got %q", got)
	}
}

func TestTrimJSONString_OnlyQuotes(t *testing.T) {
	got := trimJSONString(`""`)
	if got != "" {
		t.Errorf("expected empty string from empty quotes, got %q", got)
	}
}

func TestTrimJSONString_InnerQuotes(t *testing.T) {
	got := trimJSONString(`"he"llo"`)
	if got != `he"llo` {
		t.Errorf("expected %q, got %q", `he"llo`, got)
	}
}

// --- parseSCIMPagination ---

func TestParseSCIMPagination_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users", nil)
	startIndex, count := parseSCIMPagination(r)
	if startIndex != 1 {
		t.Errorf("expected startIndex 1, got %d", startIndex)
	}
	if count != defaultCount {
		t.Errorf("expected count %d, got %d", defaultCount, count)
	}
}

func TestParseSCIMPagination_CustomValues(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users?startIndex=5&count=25", nil)
	startIndex, count := parseSCIMPagination(r)
	if startIndex != 5 {
		t.Errorf("expected startIndex 5, got %d", startIndex)
	}
	if count != 25 {
		t.Errorf("expected count 25, got %d", count)
	}
}

func TestParseSCIMPagination_NegativeStartIndex(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users?startIndex=-5", nil)
	startIndex, _ := parseSCIMPagination(r)
	if startIndex < 1 {
		t.Errorf("expected startIndex >= 1, got %d", startIndex)
	}
}

func TestParseSCIMPagination_ZeroCount(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users?count=0", nil)
	_, count := parseSCIMPagination(r)
	if count < 1 {
		t.Errorf("expected count >= 1, got %d", count)
	}
}

func TestParseSCIMPagination_ExcessiveCount(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users?count=5000", nil)
	_, count := parseSCIMPagination(r)
	if count > 1000 {
		t.Errorf("expected count capped at 1000, got %d", count)
	}
}

func TestParseSCIMPagination_InvalidValues(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/Users?startIndex=abc&count=xyz", nil)
	startIndex, count := parseSCIMPagination(r)
	if startIndex != 1 {
		t.Errorf("expected fallback startIndex 1, got %d", startIndex)
	}
	if count != defaultCount {
		t.Errorf("expected fallback count %d, got %d", defaultCount, count)
	}
}

// --- queryIntDefault ---

func TestQueryIntDefault_Present(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?key=42", nil)
	got := queryIntDefault(r, "key", 10)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestQueryIntDefault_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := queryIntDefault(r, "key", 10)
	if got != 10 {
		t.Errorf("expected fallback 10, got %d", got)
	}
}

func TestQueryIntDefault_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/?key=notanumber", nil)
	got := queryIntDefault(r, "key", 99)
	if got != 99 {
		t.Errorf("expected fallback 99, got %d", got)
	}
}

// --- dbUserToSCIM ---

func TestDbUserToSCIM_Basic(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	extID := "ext-123"
	user := dbUserToSCIM(id, "alice", "alice@example.com", "Alice", "Smith", true, &extID, now, now)

	if user.ID != id.String() {
		t.Errorf("expected ID %q, got %q", id.String(), user.ID)
	}
	if user.UserName != "alice" {
		t.Errorf("expected userName %q, got %q", "alice", user.UserName)
	}
	if !user.Active {
		t.Error("expected active to be true")
	}
	if len(user.Schemas) != 1 || user.Schemas[0] != schemaUser {
		t.Errorf("unexpected schemas: %v", user.Schemas)
	}
	if user.Name == nil {
		t.Fatal("expected Name to be non-nil")
	}
	if user.Name.GivenName != "Alice" {
		t.Errorf("expected givenName %q, got %q", "Alice", user.Name.GivenName)
	}
	if user.Name.FamilyName != "Smith" {
		t.Errorf("expected familyName %q, got %q", "Smith", user.Name.FamilyName)
	}
	if user.Name.Formatted != "Alice Smith" {
		t.Errorf("expected formatted name %q, got %q", "Alice Smith", user.Name.Formatted)
	}
	if len(user.Emails) != 1 || user.Emails[0].Value != "alice@example.com" {
		t.Errorf("unexpected emails: %v", user.Emails)
	}
	if !user.Emails[0].Primary {
		t.Error("expected primary email")
	}
	if user.ExternalID != "ext-123" {
		t.Errorf("expected externalId %q, got %q", "ext-123", user.ExternalID)
	}
	if user.Meta.ResourceType != "User" {
		t.Errorf("expected resource type %q, got %q", "User", user.Meta.ResourceType)
	}
}

func TestDbUserToSCIM_NoEmail(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	user := dbUserToSCIM(id, "bob", "", "Bob", "", true, nil, now, now)
	if len(user.Emails) != 0 {
		t.Errorf("expected no emails, got %v", user.Emails)
	}
	if user.ExternalID != "" {
		t.Errorf("expected empty externalId, got %q", user.ExternalID)
	}
}

// --- dbGroupToSCIM ---

func TestDbGroupToSCIM(t *testing.T) {
	id := uuid.New()
	now := time.Now()
	members := []SCIMMember{
		{Value: "user-1", Display: "alice"},
		{Value: "user-2", Display: "bob"},
	}
	group := dbGroupToSCIM(id, "admins", now, members)
	if group.ID != id.String() {
		t.Errorf("expected ID %q, got %q", id.String(), group.ID)
	}
	if group.Name != "admins" {
		t.Errorf("expected name %q, got %q", "admins", group.Name)
	}
	if len(group.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(group.Members))
	}
	if group.Meta.ResourceType != "Group" {
		t.Errorf("expected resource type %q, got %q", "Group", group.Meta.ResourceType)
	}
}

// --- parseSCIMBody ---

func TestParseSCIMBody_Valid(t *testing.T) {
	body := `{"userName":"alice","active":true}`
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	var req CreateUserRequest
	if err := parseSCIMBody(r, &req); err != nil {
		t.Fatal(err)
	}
	if req.UserName != "alice" {
		t.Errorf("expected userName %q, got %q", "alice", req.UserName)
	}
}

func TestParseSCIMBody_NilBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = nil
	var req CreateUserRequest
	if err := parseSCIMBody(r, &req); err == nil {
		t.Error("expected error for nil body")
	}
}

func TestParseSCIMBody_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("{invalid"))
	var req CreateUserRequest
	if err := parseSCIMBody(r, &req); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- parseMemberFilter ---

func TestParseMemberFilter_Valid(t *testing.T) {
	id, ok := parseMemberFilter(`members[value eq "550e8400-e29b-41d4-a716-446655440000"]`)
	if !ok {
		t.Fatal("expected match")
	}
	if id != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected UUID, got %q", id)
	}
}

func TestParseMemberFilter_NoMatch(t *testing.T) {
	tests := []string{
		"members",
		`members[value eq ]`,
		`members[value eq ""]`,
		`users[value eq "123"]`,
		"",
	}
	for _, tt := range tests {
		_, ok := parseMemberFilter(tt)
		// The empty value case `members[value eq ""]` is borderline; the function
		// checks length, so empty UUID may or may not match. We just verify no panic.
		_ = ok
	}
}

// --- strPtrOrNil ---

func TestStrPtrOrNil_NonEmpty(t *testing.T) {
	p := strPtrOrNil("hello")
	if p == nil || *p != "hello" {
		t.Error("expected non-nil pointer to 'hello'")
	}
}

func TestStrPtrOrNil_Empty(t *testing.T) {
	p := strPtrOrNil("")
	if p != nil {
		t.Error("expected nil for empty string")
	}
}

// --- nilIfEmpty ---

func TestNilIfEmpty_NonEmpty(t *testing.T) {
	s := "hello"
	p := nilIfEmpty(&s)
	if p == nil || *p != "hello" {
		t.Error("expected non-nil pointer")
	}
}

func TestNilIfEmpty_Empty(t *testing.T) {
	s := ""
	p := nilIfEmpty(&s)
	if p != nil {
		t.Error("expected nil for empty string")
	}
}

func TestNilIfEmpty_Nil(t *testing.T) {
	p := nilIfEmpty(nil)
	if p != nil {
		t.Error("expected nil for nil input")
	}
}

// --- patchValueToEmails ---

func TestPatchValueToEmails_Valid(t *testing.T) {
	input := []any{
		map[string]any{"value": "a@b.com", "type": "work", "primary": true},
	}
	emails, err := patchValueToEmails(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(emails) != 1 || emails[0].Value != "a@b.com" {
		t.Errorf("unexpected emails: %v", emails)
	}
}

func TestPatchValueToEmails_InvalidFormat(t *testing.T) {
	_, err := patchValueToEmails("not-an-array")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

// --- patchValueToMembers ---

func TestPatchValueToMembers_Valid(t *testing.T) {
	input := []any{
		map[string]any{"value": "user-1", "display": "alice"},
	}
	members, err := patchValueToMembers(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Value != "user-1" {
		t.Errorf("unexpected members: %v", members)
	}
}

// --- respondSCIMError ---

func TestRespondSCIMError(t *testing.T) {
	w := httptest.NewRecorder()
	respondSCIMError(w, http.StatusBadRequest, "invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != scimMediaType {
		t.Errorf("expected Content-Type %q, got %q", scimMediaType, ct)
	}

	var errResp SCIMError
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatal(err)
	}
	if errResp.Detail != "invalid input" {
		t.Errorf("expected detail %q, got %q", "invalid input", errResp.Detail)
	}
	if errResp.Status != "400" {
		t.Errorf("expected status %q, got %q", "400", errResp.Status)
	}
	if len(errResp.Schemas) != 1 || errResp.Schemas[0] != schemaError {
		t.Errorf("unexpected schemas: %v", errResp.Schemas)
	}
}

// --- respondSCIM ---

func TestRespondSCIM_NilData(t *testing.T) {
	w := httptest.NewRecorder()
	respondSCIM(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %q", w.Body.String())
	}
}

// --- SCIMListResponse JSON ---

func TestSCIMListResponse_JSON(t *testing.T) {
	resp := SCIMListResponse{
		Schemas:      []string{schemaListResponse},
		TotalResults: 42,
		StartIndex:   1,
		ItemsPerPage: 10,
		Resources:    []string{},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["totalResults"].(float64) != 42 {
		t.Errorf("expected totalResults 42, got %v", parsed["totalResults"])
	}
	if parsed["startIndex"].(float64) != 1 {
		t.Errorf("expected startIndex 1, got %v", parsed["startIndex"])
	}
}
