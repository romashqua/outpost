package saml

import (
	"testing"
	"time"
)

func TestIsValidNameID_ValidEmail(t *testing.T) {
	if !isValidNameID("alice@example.com") {
		t.Error("expected valid email NameID")
	}
}

func TestIsValidNameID_ValidAlphanumeric(t *testing.T) {
	if !isValidNameID("user123") {
		t.Error("expected alphanumeric NameID to be valid")
	}
}

func TestIsValidNameID_AllowedSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"user@domain.com", "at sign and dot"},
		{"first.last", "dot"},
		{"user_name", "underscore"},
		{"user+tag", "plus"},
		{"user-name", "hyphen"},
		{"abc=def", "equals"},
		{"path/to/resource", "forward slash"},
	}
	for _, tt := range tests {
		if !isValidNameID(tt.input) {
			t.Errorf("expected %q (%s) to be valid", tt.input, tt.desc)
		}
	}
}

func TestIsValidNameID_Empty(t *testing.T) {
	// Empty string: the loop does not execute, returns true.
	// This matches the allowlist approach (no forbidden chars found).
	if !isValidNameID("") {
		t.Error("expected empty string to return true (no invalid chars)")
	}
}

func TestIsValidNameID_InvalidChars(t *testing.T) {
	tests := []struct {
		input string
		desc  string
	}{
		{"<script>", "angle brackets"},
		{"user&admin", "ampersand"},
		{"hello world", "space"},
		{"user;drop", "semicolon"},
		{"alert('xss')", "parentheses and quotes"},
		{"name\ttab", "tab character"},
		{"new\nline", "newline"},
		{`"quoted"`, "double quotes"},
		{"user#tag", "hash"},
		{"100%done", "percent"},
		{"a!b", "exclamation mark"},
		{"a$b", "dollar sign"},
		{"a^b", "caret"},
		{"a*b", "asterisk"},
		{"a{b}", "curly braces"},
		{"a[b]", "square brackets"},
		{"a|b", "pipe"},
		{"a\\b", "backslash"},
		{"a~b", "tilde"},
		{"a`b", "backtick"},
	}
	for _, tt := range tests {
		if isValidNameID(tt.input) {
			t.Errorf("expected %q (%s) to be invalid", tt.input, tt.desc)
		}
	}
}

func TestIsValidNameID_OnlySpecialChars(t *testing.T) {
	if !isValidNameID("@._+-=/") {
		t.Error("expected string of only allowed special chars to be valid")
	}
}

func TestIsValidNameID_Unicode(t *testing.T) {
	// Unicode letters outside ASCII are not in the allowlist.
	if isValidNameID("user\u00e9name") {
		t.Error("expected non-ASCII unicode to be invalid")
	}
}

func TestIsValidNameID_LongValid(t *testing.T) {
	// 256 'a' characters should be valid.
	long := ""
	for i := 0; i < 256; i++ {
		long += "a"
	}
	if !isValidNameID(long) {
		t.Error("expected long alphanumeric string to be valid")
	}
}

func TestDefaultAttributeMap(t *testing.T) {
	m := DefaultAttributeMap()
	if m.Email != "urn:oid:0.9.2342.19200300.100.1.3" {
		t.Errorf("unexpected default email attr: %q", m.Email)
	}
	if m.FirstName != "givenName" {
		t.Errorf("unexpected default first name attr: %q", m.FirstName)
	}
	if m.LastName != "sn" {
		t.Errorf("unexpected default last name attr: %q", m.LastName)
	}
	if m.Username != "uid" {
		t.Errorf("unexpected default username attr: %q", m.Username)
	}
	if m.Groups != "memberOf" {
		t.Errorf("unexpected default groups attr: %q", m.Groups)
	}
}

func TestMatchesAttr_ConfiguredMatch(t *testing.T) {
	if !matchesAttr("urn:oid:0.9.2342.19200300.100.1.3", "", "urn:oid:0.9.2342.19200300.100.1.3", "email") {
		t.Error("expected name match against configured value")
	}
}

func TestMatchesAttr_FriendlyNameMatch(t *testing.T) {
	if !matchesAttr("some:oid", "email", "email", "fallback") {
		t.Error("expected friendly name match against configured value")
	}
}

func TestMatchesAttr_FallbackMatch(t *testing.T) {
	if !matchesAttr("email", "", "", "email") {
		t.Error("expected name match against fallback")
	}
	if !matchesAttr("other", "email", "", "email") {
		t.Error("expected friendly name match against fallback")
	}
}

func TestMatchesAttr_NoMatch(t *testing.T) {
	if matchesAttr("something", "else", "configured", "fallback") {
		t.Error("expected no match")
	}
}

func TestValidUntil(t *testing.T) {
	before := ValidUntil()
	// Should be approximately 24 hours from now.
	diff := before.Sub(time.Now().UTC())
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("expected ValidUntil ~24h from now, got %v", diff)
	}
}
