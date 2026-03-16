package ldap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"testing"
)

// --- Entry tests ---

func TestEntry_GetAttributeValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		entry  Entry
		attr   string
		expect string
	}{
		{
			name: "existing attribute with single value",
			entry: Entry{
				DN:         "cn=test",
				Attributes: map[string][]string{"mail": {"alice@example.com"}},
			},
			attr:   "mail",
			expect: "alice@example.com",
		},
		{
			name: "existing attribute with multiple values returns first",
			entry: Entry{
				DN:         "cn=test",
				Attributes: map[string][]string{"mail": {"first@example.com", "second@example.com"}},
			},
			attr:   "mail",
			expect: "first@example.com",
		},
		{
			name: "missing attribute returns empty string",
			entry: Entry{
				DN:         "cn=test",
				Attributes: map[string][]string{"mail": {"alice@example.com"}},
			},
			attr:   "phone",
			expect: "",
		},
		{
			name: "nil attributes map returns empty string",
			entry: Entry{
				DN: "cn=test",
			},
			attr:   "mail",
			expect: "",
		},
		{
			name: "empty values slice returns empty string",
			entry: Entry{
				DN:         "cn=test",
				Attributes: map[string][]string{"mail": {}},
			},
			attr:   "mail",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.entry.GetAttributeValue(tt.attr)
			if got != tt.expect {
				t.Errorf("GetAttributeValue(%q) = %q, want %q", tt.attr, got, tt.expect)
			}
		})
	}
}

func TestEntry_GetAttributeValues(t *testing.T) {
	t.Parallel()
	entry := Entry{
		DN: "cn=test",
		Attributes: map[string][]string{
			"member": {"cn=alice,dc=example", "cn=bob,dc=example"},
		},
	}

	vals := entry.GetAttributeValues("member")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if vals[0] != "cn=alice,dc=example" || vals[1] != "cn=bob,dc=example" {
		t.Errorf("unexpected values: %v", vals)
	}

	nilVals := entry.GetAttributeValues("nonexistent")
	if nilVals != nil {
		t.Errorf("expected nil for nonexistent attr, got %v", nilVals)
	}
}

// --- Default attribute maps ---

func TestDefaultUserAttrMap(t *testing.T) {
	t.Parallel()
	m := DefaultUserAttrMap()
	if m.Username != "sAMAccountName" {
		t.Errorf("Username = %q, want sAMAccountName", m.Username)
	}
	if m.Email != "mail" {
		t.Errorf("Email = %q, want mail", m.Email)
	}
	if m.FirstName != "givenName" {
		t.Errorf("FirstName = %q, want givenName", m.FirstName)
	}
	if m.LastName != "sn" {
		t.Errorf("LastName = %q, want sn", m.LastName)
	}
	if m.Phone != "telephoneNumber" {
		t.Errorf("Phone = %q, want telephoneNumber", m.Phone)
	}
	if m.DN != "dn" {
		t.Errorf("DN = %q, want dn", m.DN)
	}
}

func TestDefaultGroupAttrMap(t *testing.T) {
	t.Parallel()
	m := DefaultGroupAttrMap()
	if m.Name != "cn" {
		t.Errorf("Name = %q, want cn", m.Name)
	}
	if m.Members != "member" {
		t.Errorf("Members = %q, want member", m.Members)
	}
}

// --- ldapEscapeFilter ---

func TestLdapEscapeFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input  string
		expect string
	}{
		{"simple", "simple"},
		{"", ""},
		{"has*wildcard", `has\2awildcard`},
		{"has(paren)", `has\28paren\29`},
		{`back\slash`, `back\5cslash`},
		{"null\x00byte", `null\00byte`},
		{`all*()\`, `all\2a\28\29\5c`},
		{"no_special_chars_123", "no_special_chars_123"},
		{"user@domain.com", "user@domain.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := ldapEscapeFilter(tt.input)
			if got != tt.expect {
				t.Errorf("ldapEscapeFilter(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

// --- Mock connector for Syncer tests ---

type mockConnector struct {
	connectErr error
	bindErr    error
	searchErr  error
	entries    []*Entry
	closeErr   error

	connectCalls int
	bindCalls    int
	searchCalls  int
	closeCalls   int

	lastBindDN       string
	lastBindPassword string
	lastSearchFilter string
}

func (m *mockConnector) Connect(url string, tlsCfg *tls.Config) error {
	m.connectCalls++
	return m.connectErr
}

func (m *mockConnector) Bind(dn, password string) error {
	m.bindCalls++
	m.lastBindDN = dn
	m.lastBindPassword = password
	return m.bindErr
}

func (m *mockConnector) Search(baseDN, filter string, attributes []string) ([]*Entry, error) {
	m.searchCalls++
	m.lastSearchFilter = filter
	return m.entries, m.searchErr
}

func (m *mockConnector) Close() error {
	m.closeCalls++
	return m.closeErr
}

// --- NewSyncer tests ---

func TestNewSyncer_AppliesDefaults(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
	}, nil, logger)

	// Should have filled in defaults for UserAttrMap, GroupAttrMap, filters.
	if s.cfg.UserAttrMap.Username != "sAMAccountName" {
		t.Errorf("expected default UserAttrMap.Username, got %q", s.cfg.UserAttrMap.Username)
	}
	if s.cfg.GroupAttrMap.Name != "cn" {
		t.Errorf("expected default GroupAttrMap.Name, got %q", s.cfg.GroupAttrMap.Name)
	}
	if s.cfg.UserFilter != "(objectClass=person)" {
		t.Errorf("expected default UserFilter, got %q", s.cfg.UserFilter)
	}
	if s.cfg.GroupFilter != "(objectClass=group)" {
		t.Errorf("expected default GroupFilter, got %q", s.cfg.GroupFilter)
	}
}

func TestNewSyncer_PreservesCustomConfig(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	customUserMap := UserAttributeMap{
		Username:  "uid",
		Email:     "userEmail",
		FirstName: "gn",
		LastName:  "surname",
		Phone:     "tel",
		DN:        "dn",
	}
	customGroupMap := GroupAttributeMap{
		Name:    "groupName",
		Members: "memberUid",
	}

	s := NewSyncer(Config{
		URL:          "ldap://localhost:389",
		BindDN:       "cn=admin,dc=test",
		UserFilter:   "(objectClass=inetOrgPerson)",
		GroupFilter:  "(objectClass=posixGroup)",
		UserAttrMap:  customUserMap,
		GroupAttrMap: customGroupMap,
	}, nil, logger)

	if s.cfg.UserAttrMap.Username != "uid" {
		t.Errorf("expected custom Username attr, got %q", s.cfg.UserAttrMap.Username)
	}
	if s.cfg.GroupAttrMap.Name != "groupName" {
		t.Errorf("expected custom GroupAttrMap.Name, got %q", s.cfg.GroupAttrMap.Name)
	}
	if s.cfg.UserFilter != "(objectClass=inetOrgPerson)" {
		t.Errorf("expected custom UserFilter, got %q", s.cfg.UserFilter)
	}
}

// --- TLSEnabled / tlsConfig ---

func TestSyncer_TLSEnabled(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{TLS: false}, nil, logger)
	if s.TLSEnabled() {
		t.Error("expected TLSEnabled() = false")
	}

	s2 := NewSyncer(Config{TLS: true}, nil, logger)
	if !s2.TLSEnabled() {
		t.Error("expected TLSEnabled() = true")
	}
}

func TestSyncer_TlsConfig_Nil_When_Disabled(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{TLS: false}, nil, logger)
	if s.tlsConfig() != nil {
		t.Error("expected nil tls config when TLS is disabled")
	}
}

func TestSyncer_TlsConfig_SkipVerify(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{TLS: true, SkipVerify: true}, nil, logger)
	tlsCfg := s.tlsConfig()
	if tlsCfg == nil {
		t.Fatal("expected non-nil tls config")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify = true")
	}
}

// --- entryToUser ---

func TestSyncer_EntryToUser(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{}, nil, logger)
	entry := &Entry{
		DN: "cn=alice,dc=example,dc=com",
		Attributes: map[string][]string{
			"sAMAccountName": {"alice"},
			"mail":           {"alice@example.com"},
			"givenName":      {"Alice"},
			"sn":             {"Smith"},
			"telephoneNumber": {"+1234567890"},
		},
	}

	user := s.entryToUser(entry)
	if user.DN != "cn=alice,dc=example,dc=com" {
		t.Errorf("DN = %q", user.DN)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %q", user.Username)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q", user.Email)
	}
	if user.FirstName != "Alice" {
		t.Errorf("FirstName = %q", user.FirstName)
	}
	if user.LastName != "Smith" {
		t.Errorf("LastName = %q", user.LastName)
	}
	if user.Phone != "+1234567890" {
		t.Errorf("Phone = %q", user.Phone)
	}
}

func TestSyncer_EntryToUser_MissingAttributes(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{}, nil, logger)
	entry := &Entry{
		DN:         "cn=bob,dc=example,dc=com",
		Attributes: map[string][]string{},
	}

	user := s.entryToUser(entry)
	if user.DN != "cn=bob,dc=example,dc=com" {
		t.Errorf("DN = %q", user.DN)
	}
	if user.Username != "" {
		t.Errorf("expected empty Username, got %q", user.Username)
	}
	if user.Email != "" {
		t.Errorf("expected empty Email, got %q", user.Email)
	}
}

// --- userSearchAttrs ---

func TestSyncer_UserSearchAttrs(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{}, nil, logger)
	attrs := s.userSearchAttrs()

	// Should contain Username, Email, FirstName, LastName, Phone
	expected := map[string]bool{
		"sAMAccountName": true,
		"mail":           true,
		"givenName":      true,
		"sn":             true,
		"telephoneNumber": true,
	}
	if len(attrs) != len(expected) {
		t.Fatalf("expected %d attrs, got %d: %v", len(expected), len(attrs), attrs)
	}
	for _, a := range attrs {
		if !expected[a] {
			t.Errorf("unexpected attribute %q in search attrs", a)
		}
	}
}

func TestSyncer_UserSearchAttrs_NoPhone(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s := NewSyncer(Config{
		UserAttrMap: UserAttributeMap{
			Username:  "uid",
			Email:     "mail",
			FirstName: "gn",
			LastName:  "sn",
			Phone:     "", // empty phone means no phone attr
			DN:        "dn",
		},
	}, nil, logger)
	attrs := s.userSearchAttrs()
	if len(attrs) != 4 {
		t.Fatalf("expected 4 attrs (no phone), got %d: %v", len(attrs), attrs)
	}
}

// --- TestConnection with mock ---

func TestSyncer_TestConnection_Success(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{}
	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
	}, nil, logger)
	s.SetConnector(mock)

	err := s.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.connectCalls != 1 {
		t.Errorf("expected 1 connect call, got %d", mock.connectCalls)
	}
	if mock.bindCalls != 1 {
		t.Errorf("expected 1 bind call, got %d", mock.bindCalls)
	}
	if mock.closeCalls != 1 {
		t.Errorf("expected 1 close call, got %d", mock.closeCalls)
	}
}

func TestSyncer_TestConnection_ConnectError(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{connectErr: fmt.Errorf("connection refused")}
	s := NewSyncer(Config{URL: "ldap://bad:389"}, nil, logger)
	s.SetConnector(mock)

	err := s.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSyncer_TestConnection_BindError(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{bindErr: fmt.Errorf("invalid credentials")}
	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
	}, nil, logger)
	s.SetConnector(mock)

	err := s.TestConnection(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Should have closed after bind failure.
	if mock.closeCalls != 1 {
		t.Errorf("expected 1 close call after bind failure, got %d", mock.closeCalls)
	}
}

// --- Authenticate with mock ---

func TestSyncer_Authenticate_Success(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{
		entries: []*Entry{
			{
				DN: "cn=alice,dc=example,dc=com",
				Attributes: map[string][]string{
					"sAMAccountName": {"alice"},
					"mail":           {"alice@example.com"},
					"givenName":      {"Alice"},
					"sn":             {"Smith"},
				},
			},
		},
	}

	s := NewSyncer(Config{
		URL:        "ldap://localhost:389",
		BindDN:     "cn=admin,dc=test",
		BaseDN:     "dc=example,dc=com",
		UserFilter: "(objectClass=person)",
	}, nil, logger)
	s.SetConnector(mock)

	user, err := s.Authenticate(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("Username = %q, want alice", user.Username)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("Email = %q, want alice@example.com", user.Email)
	}
	// Should have bound twice: once as service account, once as user.
	if mock.bindCalls != 2 {
		t.Errorf("expected 2 bind calls, got %d", mock.bindCalls)
	}
	if mock.lastBindDN != "cn=alice,dc=example,dc=com" {
		t.Errorf("last bind DN = %q, want cn=alice,dc=example,dc=com", mock.lastBindDN)
	}
}

func TestSyncer_Authenticate_UserNotFound(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{
		entries: []*Entry{}, // no results
	}

	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
		BaseDN: "dc=example,dc=com",
	}, nil, logger)
	s.SetConnector(mock)

	_, err := s.Authenticate(context.Background(), "nobody", "pass")
	if err == nil {
		t.Fatal("expected error for not found user")
	}
}

func TestSyncer_Authenticate_MultipleResults(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockConnector{
		entries: []*Entry{
			{DN: "cn=alice1,dc=test", Attributes: map[string][]string{"sAMAccountName": {"alice"}}},
			{DN: "cn=alice2,dc=test", Attributes: map[string][]string{"sAMAccountName": {"alice"}}},
		},
	}

	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
		BaseDN: "dc=test",
	}, nil, logger)
	s.SetConnector(mock)

	_, err := s.Authenticate(context.Background(), "alice", "pass")
	if err == nil {
		t.Fatal("expected error for multiple entries")
	}
}

func TestSyncer_Authenticate_BindAsUserFails(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	bindCall := 0
	mock := &mockConnector{
		entries: []*Entry{
			{
				DN: "cn=alice,dc=test",
				Attributes: map[string][]string{
					"sAMAccountName": {"alice"},
					"mail":           {"alice@test.com"},
				},
			},
		},
	}
	// Make the second bind (user bind) fail.
	origBind := mock.bindErr
	_ = origBind

	s := NewSyncer(Config{
		URL:    "ldap://localhost:389",
		BindDN: "cn=admin,dc=test",
		BaseDN: "dc=test",
	}, nil, logger)

	// Custom mock that fails on second bind.
	failOnUserBind := &bindFailOnSecondCallConnector{
		base: mock,
	}
	s.SetConnector(failOnUserBind)
	_ = bindCall

	_, err := s.Authenticate(context.Background(), "alice", "wrongpassword")
	if err == nil {
		t.Fatal("expected error for bad password")
	}
}

// bindFailOnSecondCallConnector wraps a connector and fails on the second Bind call.
type bindFailOnSecondCallConnector struct {
	base      *mockConnector
	bindCalls int
}

func (c *bindFailOnSecondCallConnector) Connect(url string, tlsCfg *tls.Config) error {
	return c.base.Connect(url, tlsCfg)
}

func (c *bindFailOnSecondCallConnector) Bind(dn, password string) error {
	c.bindCalls++
	if c.bindCalls >= 2 {
		return fmt.Errorf("invalid credentials")
	}
	return c.base.Bind(dn, password)
}

func (c *bindFailOnSecondCallConnector) Search(baseDN, filter string, attributes []string) ([]*Entry, error) {
	return c.base.Search(baseDN, filter, attributes)
}

func (c *bindFailOnSecondCallConnector) Close() error {
	return c.base.Close()
}

// --- LDAPConnector unit tests (non-network) ---

func TestLDAPConnector_SetPageSize(t *testing.T) {
	t.Parallel()
	c := NewLDAPConnector()
	if c.pageSize != defaultPageSize {
		t.Errorf("default pageSize = %d, want %d", c.pageSize, defaultPageSize)
	}

	c.SetPageSize(1000)
	if c.pageSize != 1000 {
		t.Errorf("pageSize after Set = %d, want 1000", c.pageSize)
	}

	c.SetPageSize(0)
	if c.pageSize != 0 {
		t.Errorf("pageSize after Set(0) = %d, want 0", c.pageSize)
	}
}

func TestLDAPConnector_BindWithoutConnect(t *testing.T) {
	t.Parallel()
	c := NewLDAPConnector()
	err := c.Bind("cn=admin", "password")
	if err == nil {
		t.Fatal("expected error when binding without connection")
	}
}

func TestLDAPConnector_SearchWithoutConnect(t *testing.T) {
	t.Parallel()
	c := NewLDAPConnector()
	_, err := c.Search("dc=test", "(objectClass=*)", nil)
	if err == nil {
		t.Fatal("expected error when searching without connection")
	}
}

func TestLDAPConnector_CloseWithoutConnect(t *testing.T) {
	t.Parallel()
	c := NewLDAPConnector()
	err := c.Close()
	if err != nil {
		t.Fatalf("Close without connection should not error: %v", err)
	}
}

// --- SyncResult type ---

func TestSyncResult_ZeroValue(t *testing.T) {
	t.Parallel()
	r := SyncResult{}
	if r.Created != 0 || r.Updated != 0 || r.Disabled != 0 {
		t.Error("zero value should have zero counts")
	}
	if r.Errors != nil {
		t.Error("zero value Errors should be nil")
	}
}
