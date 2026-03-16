package mfa

import (
	"strings"
	"testing"
	"time"
)

// --- TOTPManager ---

func TestTOTPManager_GenerateSecret(t *testing.T) {
	mgr := NewTOTPManager()
	secret, qrURL, qrImage, err := mgr.GenerateSecret("Outpost VPN", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Error("expected non-empty secret")
	}
	if !strings.HasPrefix(qrURL, "otpauth://totp/") {
		t.Errorf("expected otpauth URL, got %q", qrURL)
	}
	if !strings.HasPrefix(qrImage, "data:image/png;base64,") {
		t.Errorf("expected base64 PNG data URI, got prefix: %q", qrImage[:30])
	}
	if !strings.Contains(qrURL, "Outpost+VPN") && !strings.Contains(qrURL, "Outpost%20VPN") {
		t.Errorf("expected issuer in URL, got %q", qrURL)
	}
}

func TestTOTPManager_Validate_InvalidCode(t *testing.T) {
	mgr := NewTOTPManager()
	secret, _, _, err := mgr.GenerateSecret("Test", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}
	// An arbitrary wrong code should not validate.
	if mgr.Validate(secret, "000000") {
		// There's a very small chance this might be the current valid code,
		// but for practical purposes this is fine.
		t.Log("warning: 000000 unexpectedly validated (rare timing coincidence)")
	}
	if mgr.Validate(secret, "") {
		t.Error("empty code should not validate")
	}
	if mgr.Validate(secret, "not-a-number") {
		t.Error("non-numeric code should not validate")
	}
}

func TestTOTPManager_DifferentSecrets(t *testing.T) {
	mgr := NewTOTPManager()
	s1, _, _, _ := mgr.GenerateSecret("Test", "a@b.com")
	s2, _, _, _ := mgr.GenerateSecret("Test", "c@d.com")
	if s1 == s2 {
		t.Error("expected different secrets for different accounts")
	}
}

// --- EmailTokenManager ---

func TestEmailTokenManager_SendAndValidate(t *testing.T) {
	mgr := NewEmailTokenManager(nil)

	code, err := mgr.SendToken("user-1", "user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Errorf("expected 6-digit code, got %q (len=%d)", code, len(code))
	}
	// Validate with correct code.
	if !mgr.ValidateToken("user-1", code) {
		t.Error("expected valid token")
	}
	// Token should be consumed (single-use).
	if mgr.ValidateToken("user-1", code) {
		t.Error("expected token to be consumed after first use")
	}
}

func TestEmailTokenManager_WrongCode(t *testing.T) {
	mgr := NewEmailTokenManager(nil)
	_, err := mgr.SendToken("user-2", "user2@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if mgr.ValidateToken("user-2", "wrong!") {
		t.Error("expected wrong code to fail validation")
	}
}

func TestEmailTokenManager_WrongUser(t *testing.T) {
	mgr := NewEmailTokenManager(nil)
	code, _ := mgr.SendToken("user-3", "user3@example.com")
	if mgr.ValidateToken("other-user", code) {
		t.Error("expected validation to fail for wrong user")
	}
}

func TestEmailTokenManager_NoToken(t *testing.T) {
	mgr := NewEmailTokenManager(nil)
	if mgr.ValidateToken("no-such-user", "123456") {
		t.Error("expected validation to fail when no token exists")
	}
}

func TestEmailTokenManager_OverwritesPreviousToken(t *testing.T) {
	mgr := NewEmailTokenManager(nil)
	code1, _ := mgr.SendToken("user-4", "user4@example.com")
	code2, _ := mgr.SendToken("user-4", "user4@example.com")

	// Old code should no longer work.
	if code1 == code2 {
		t.Log("codes happen to be the same (unlikely)")
	}
	if mgr.ValidateToken("user-4", code1) && code1 != code2 {
		t.Error("expected old token to be overwritten")
	}
}

// --- generateDigitCode ---

func TestGenerateDigitCode_Length(t *testing.T) {
	for _, length := range []int{4, 6, 8, 10} {
		code, err := generateDigitCode(length)
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != length {
			t.Errorf("expected length %d, got %d", length, len(code))
		}
	}
}

func TestGenerateDigitCode_OnlyDigits(t *testing.T) {
	code, err := generateDigitCode(100)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			t.Errorf("expected digit, got %c", c)
		}
	}
}

func TestGenerateDigitCode_Unique(t *testing.T) {
	c1, _ := generateDigitCode(20)
	c2, _ := generateDigitCode(20)
	if c1 == c2 {
		t.Error("expected two random codes to differ")
	}
}

// --- randomString ---

func TestRandomString_Length(t *testing.T) {
	s, err := randomString(8, backupCodeChars)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 8 {
		t.Errorf("expected length 8, got %d", len(s))
	}
}

func TestRandomString_Alphabet(t *testing.T) {
	alphabet := "abc"
	s, err := randomString(100, alphabet)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range s {
		if !strings.ContainsRune(alphabet, c) {
			t.Errorf("character %c not in alphabet %q", c, alphabet)
		}
	}
}

func TestRandomString_Unique(t *testing.T) {
	s1, _ := randomString(16, backupCodeChars)
	s2, _ := randomString(16, backupCodeChars)
	if s1 == s2 {
		t.Error("expected two random strings to differ")
	}
}

// --- emailToken expiry (indirect test via ValidateToken) ---

func TestEmailTokenManager_Expiry(t *testing.T) {
	mgr := NewEmailTokenManager(nil)

	code, _ := mgr.SendToken("user-exp", "exp@example.com")

	// Manually overwrite with an expired token.
	mgr.tokens.Store("user-exp", emailToken{
		Code:      code,
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	})

	if mgr.ValidateToken("user-exp", code) {
		t.Error("expected expired token to fail validation")
	}
}

// --- MFAStatus struct ---

func TestMFAStatus_ZeroValue(t *testing.T) {
	s := MFAStatus{}
	if s.MFAEnabled {
		t.Error("expected MFAEnabled to be false by default")
	}
	if s.TOTPConfigured {
		t.Error("expected TOTPConfigured to be false by default")
	}
	if s.BackupCodesLeft != 0 {
		t.Errorf("expected 0 backup codes, got %d", s.BackupCodesLeft)
	}
}

// --- backupCode constants ---

func TestBackupCodeConstants(t *testing.T) {
	if backupCodeCount != 10 {
		t.Errorf("expected 10 backup codes, got %d", backupCodeCount)
	}
	if backupCodeLength != 8 {
		t.Errorf("expected backup code length 8, got %d", backupCodeLength)
	}
	if len(backupCodeChars) != 36 {
		t.Errorf("expected 36 chars in alphabet (a-z + 0-9), got %d", len(backupCodeChars))
	}
}

// --- WebAuthnCredential struct ---

func TestWebAuthnCredential_ZeroValue(t *testing.T) {
	c := WebAuthnCredential{}
	if c.ID != "" {
		t.Error("expected empty ID")
	}
	if c.SignCount != 0 {
		t.Errorf("expected sign count 0, got %d", c.SignCount)
	}
	if !c.CreatedAt.IsZero() {
		t.Error("expected zero time for CreatedAt")
	}
}
