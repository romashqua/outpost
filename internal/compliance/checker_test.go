package compliance

import (
	"testing"
)

func TestCalculateOverallScore(t *testing.T) {
	c := &Checker{}

	tests := []struct {
		name       string
		checks     []ComplianceCheck
		wantScore  int
		wantMax    int
		wantPct    int
	}{
		{
			name:      "empty checks",
			checks:    nil,
			wantScore: 0,
			wantMax:   0,
			wantPct:   0,
		},
		{
			name: "all passed",
			checks: []ComplianceCheck{
				{Status: "passed"},
				{Status: "passed"},
				{Status: "passed"},
			},
			wantScore: 6,
			wantMax:   6,
			wantPct:   100,
		},
		{
			name: "all failed",
			checks: []ComplianceCheck{
				{Status: "failed"},
				{Status: "failed"},
			},
			wantScore: 0,
			wantMax:   4,
			wantPct:   0,
		},
		{
			name: "mixed",
			checks: []ComplianceCheck{
				{Status: "passed"},
				{Status: "warning"},
				{Status: "failed"},
			},
			wantScore: 3,
			wantMax:   6,
			wantPct:   50,
		},
		{
			name: "all warnings",
			checks: []ComplianceCheck{
				{Status: "warning"},
				{Status: "warning"},
			},
			wantScore: 2,
			wantMax:   4,
			wantPct:   50,
		},
		{
			name: "single passed",
			checks: []ComplianceCheck{
				{Status: "passed"},
			},
			wantScore: 2,
			wantMax:   2,
			wantPct:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &ComplianceReport{Checks: tt.checks}
			c.calculateOverallScore(report)

			if report.OverallScore != tt.wantScore {
				t.Errorf("OverallScore = %d, want %d", report.OverallScore, tt.wantScore)
			}
			if report.MaxScore != tt.wantMax {
				t.Errorf("MaxScore = %d, want %d", report.MaxScore, tt.wantMax)
			}
			if report.Percentage != tt.wantPct {
				t.Errorf("Percentage = %d, want %d", report.Percentage, tt.wantPct)
			}
		})
	}
}

func TestComplianceCheckStructFields(t *testing.T) {
	check := ComplianceCheck{
		ID:          "test-id",
		Framework:   "SOC2",
		Name:        "Test Check",
		Description: "A test compliance check",
		Status:      "passed",
		Details:     "Everything looks good",
	}

	if check.ID != "test-id" {
		t.Errorf("ID = %q, want %q", check.ID, "test-id")
	}
	if check.Framework != "SOC2" {
		t.Errorf("Framework = %q, want %q", check.Framework, "SOC2")
	}
	if check.Status != "passed" {
		t.Errorf("Status = %q, want %q", check.Status, "passed")
	}
}

func TestComplianceReportStructFields(t *testing.T) {
	report := ComplianceReport{
		OverallScore:    10,
		MaxScore:        16,
		Percentage:      62,
		MFAAdoption:     85.5,
		EncryptionRate:  100.0,
		PostureRate:     75.0,
		AuditLogEnabled: true,
		PasswordPolicy:  false,
		SessionTimeout:  true,
		Checks:          []ComplianceCheck{},
	}

	if report.OverallScore != 10 {
		t.Errorf("OverallScore = %d, want 10", report.OverallScore)
	}
	if report.MFAAdoption != 85.5 {
		t.Errorf("MFAAdoption = %f, want 85.5", report.MFAAdoption)
	}
	if !report.AuditLogEnabled {
		t.Error("AuditLogEnabled should be true")
	}
	if report.PasswordPolicy {
		t.Error("PasswordPolicy should be false")
	}
}
