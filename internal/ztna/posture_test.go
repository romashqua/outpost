package ztna

import (
	"testing"
	"time"
)

func TestEvaluate_FullyCompliant(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-1",
		OSType:            "macos",
		OSVersion:         "14.0",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   true,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-1",
		Name:                  "strict",
		NetworkID:             "net-1",
		RequireDiskEncryption: true,
		RequireScreenLock:     true,
		RequireAntivirus:      true,
		RequireFirewall:       true,
		MinOSVersion:          map[string]string{"macos": "13.0"},
		MinPostureScore:       80,
	}

	compliant, score, violations := pc.Evaluate(posture, policy)

	if !compliant {
		t.Errorf("expected compliant, got violations: %v", violations)
	}
	if score != 100 {
		t.Errorf("expected score 100, got %d", score)
	}
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %v", violations)
	}
}

func TestEvaluate_MissingDiskEncryption(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-2",
		OSType:            "windows",
		OSVersion:         "10.0.22621",
		DiskEncrypted:     false,
		ScreenLockEnabled: true,
		AntivirusActive:   true,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-1",
		Name:                  "standard",
		NetworkID:             "net-1",
		RequireDiskEncryption: true,
		RequireScreenLock:     true,
		RequireAntivirus:      false,
		RequireFirewall:       false,
	}

	compliant, score, violations := pc.Evaluate(posture, policy)

	if compliant {
		t.Error("expected non-compliant due to missing disk encryption")
	}
	if score != 75 {
		t.Errorf("expected score 75, got %d", score)
	}
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_OutdatedOSVersion(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-3",
		OSType:            "macos",
		OSVersion:         "12.5",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   true,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-2",
		Name:                  "os-check",
		NetworkID:             "net-1",
		RequireDiskEncryption: false,
		MinOSVersion:          map[string]string{"macos": "13.0", "windows": "10.0.19044"},
	}

	compliant, score, violations := pc.Evaluate(posture, policy)

	if compliant {
		t.Error("expected non-compliant due to outdated OS")
	}
	if score != 75 {
		t.Errorf("expected score 75, got %d", score)
	}
	found := false
	for _, v := range violations {
		if v == "macos version 12.5 is below minimum required 13.0" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected OS version violation in: %v", violations)
	}
}

func TestEvaluate_ScoreCalculation(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-4",
		OSType:            "linux",
		OSVersion:         "5.15",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   false,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:               "pol-3",
		Name:             "av-required",
		NetworkID:        "net-1",
		RequireAntivirus: true,
	}

	_, score, _ := pc.Evaluate(posture, policy)
	if score != 80 {
		t.Errorf("expected score 80 (100 - 20 for antivirus), got %d", score)
	}
}

func TestEvaluate_MultipleViolations(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-5",
		OSType:            "windows",
		OSVersion:         "6.1",
		DiskEncrypted:     false,
		ScreenLockEnabled: false,
		AntivirusActive:   false,
		FirewallEnabled:   false,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-4",
		Name:                  "maximum",
		NetworkID:             "net-1",
		RequireDiskEncryption: true,
		RequireScreenLock:     true,
		RequireAntivirus:      true,
		RequireFirewall:       true,
		MinOSVersion:          map[string]string{"windows": "10.0.19044"},
		MinPostureScore:       80,
	}

	compliant, score, violations := pc.Evaluate(posture, policy)

	if compliant {
		t.Error("expected non-compliant with multiple violations")
	}
	if score != 0 {
		t.Errorf("expected score 0 (clamped), got %d", score)
	}
	// 5 direct violations + 1 score threshold violation = 6
	if len(violations) != 6 {
		t.Errorf("expected 6 violations, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_NoRequirements(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:  "dev-empty",
		OSType:    "linux",
		OSVersion: "5.0",
	}
	policy := PosturePolicy{
		ID:   "pol-empty",
		Name: "Permissive",
	}

	compliant, score, violations := pc.Evaluate(posture, policy)

	if !compliant {
		t.Errorf("expected compliant with no requirements, got violations: %v", violations)
	}
	if score != 100 {
		t.Errorf("expected score 100 with no requirements, got %d", score)
	}
}

func TestEvaluate_OSVersionExactMatch(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-exact",
		OSType:            "macos",
		OSVersion:         "13.0",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   true,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-exact",
		Name:                  "exact-os",
		RequireDiskEncryption: true,
		MinOSVersion:          map[string]string{"macos": "13.0"},
	}

	compliant, score, violations := pc.Evaluate(posture, policy)
	if !compliant {
		t.Errorf("expected compliant when OS version exactly matches minimum, violations: %v", violations)
	}
	if score != 100 {
		t.Errorf("expected score 100, got %d", score)
	}
}

func TestEvaluate_OSTypeNotInPolicy(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-linux",
		OSType:            "linux",
		OSVersion:         "5.0",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   true,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:                    "pol-os",
		Name:                  "os-policy",
		RequireDiskEncryption: true,
		MinOSVersion:          map[string]string{"macos": "13.0", "windows": "10.0"},
	}

	compliant, score, _ := pc.Evaluate(posture, policy)
	if !compliant {
		t.Error("expected compliant when OS type is not in policy MinOSVersion map")
	}
	if score != 100 {
		t.Errorf("expected score 100, got %d", score)
	}
}

func TestEvaluate_MinPostureScoreThreshold(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-threshold",
		OSType:            "macos",
		OSVersion:         "14.0",
		DiskEncrypted:     true,
		ScreenLockEnabled: true,
		AntivirusActive:   false,
		FirewallEnabled:   true,
		LastChecked:       time.Now(),
	}

	policy := PosturePolicy{
		ID:               "pol-threshold",
		Name:             "high-bar",
		RequireAntivirus: true,
		MinPostureScore:  90,
	}

	compliant, score, violations := pc.Evaluate(posture, policy)
	if compliant {
		t.Error("expected non-compliant when score is below MinPostureScore")
	}
	if score != 80 {
		t.Errorf("expected score 80 (100-20), got %d", score)
	}
	// Should have antivirus violation + score threshold violation.
	if len(violations) != 2 {
		t.Errorf("expected 2 violations, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_ScreenLockOnly(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:          "dev-sl",
		OSType:            "windows",
		OSVersion:         "11.0",
		ScreenLockEnabled: false,
	}

	policy := PosturePolicy{
		ID:                "pol-sl",
		Name:              "screen-lock-only",
		RequireScreenLock: true,
	}

	_, score, violations := pc.Evaluate(posture, policy)
	if score != 85 {
		t.Errorf("expected score 85 (100-15), got %d", score)
	}
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestEvaluate_FirewallOnly(t *testing.T) {
	pc := NewPostureChecker()

	posture := DevicePosture{
		DeviceID:        "dev-fw",
		OSType:          "linux",
		OSVersion:       "6.0",
		FirewallEnabled: false,
	}

	policy := PosturePolicy{
		ID:              "pol-fw",
		Name:            "firewall-only",
		RequireFirewall: true,
	}

	_, score, _ := pc.Evaluate(posture, policy)
	if score != 85 {
		t.Errorf("expected score 85 (100-15), got %d", score)
	}
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual   string
		required string
		want     bool
	}{
		{"14.0", "13.0", true},
		{"13.0", "13.0", true},
		{"12.5", "13.0", false},
		{"10.0.22621", "10.0.19044", true},
		{"10.0.19044", "10.0.19044", true},
		{"10.0.18000", "10.0.19044", false},
		{"6.1", "10.0", false},
		{"11.0", "10.0.19044", true},
	}

	for _, tt := range tests {
		got := isVersionAtLeast(tt.actual, tt.required)
		if got != tt.want {
			t.Errorf("isVersionAtLeast(%q, %q) = %v, want %v",
				tt.actual, tt.required, got, tt.want)
		}
	}
}
