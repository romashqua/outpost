package compliance

import (
	"testing"
)

func TestCalculateOverallScore_AllPassed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "passed"},
			{ID: "c2", Status: "passed"},
			{ID: "c3", Status: "passed"},
		},
	}

	c.calculateOverallScore(report)
	// 3 checks * 2 points = 6, max = 6, percentage = (6*100)/6 = 100
	if report.Percentage != 100 {
		t.Errorf("expected percentage 100 for all passed, got %d", report.Percentage)
	}
	if report.OverallScore != 6 {
		t.Errorf("expected overall_score 6, got %d", report.OverallScore)
	}
	if report.MaxScore != 6 {
		t.Errorf("expected max_score 6, got %d", report.MaxScore)
	}
}

func TestCalculateOverallScore_AllFailed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "failed"},
			{ID: "c2", Status: "failed"},
			{ID: "c3", Status: "failed"},
		},
	}

	c.calculateOverallScore(report)
	// 0 points, max = 6, percentage = 0
	if report.Percentage != 0 {
		t.Errorf("expected percentage 0 for all failed, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_AllWarning(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "warning"},
			{ID: "c2", Status: "warning"},
			{ID: "c3", Status: "warning"},
			{ID: "c4", Status: "warning"},
		},
	}

	c.calculateOverallScore(report)
	// 4 warnings * 1 point = 4, max = 8, percentage = (4*100)/8 = 50
	if report.Percentage != 50 {
		t.Errorf("expected percentage 50 for all warning, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_Mixed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "passed"},  // 2 points
			{ID: "c2", Status: "warning"}, // 1 point
			{ID: "c3", Status: "failed"},  // 0 points
			{ID: "c4", Status: "passed"},  // 2 points
		},
	}

	c.calculateOverallScore(report)
	// 5 points, max = 8, percentage = (5*100)/8 = 62
	if report.Percentage != 62 {
		t.Errorf("expected percentage 62 for mixed statuses, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_NoChecks(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{},
	}

	c.calculateOverallScore(report)
	if report.Percentage != 0 {
		t.Errorf("expected percentage 0 for no checks, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_SinglePassed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "passed"},
		},
	}

	c.calculateOverallScore(report)
	// 2 points, max = 2, percentage = 100
	if report.Percentage != 100 {
		t.Errorf("expected percentage 100 for single passed, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_SingleFailed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "failed"},
		},
	}

	c.calculateOverallScore(report)
	if report.Percentage != 0 {
		t.Errorf("expected percentage 0 for single failed, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_SingleWarning(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "warning"},
		},
	}

	c.calculateOverallScore(report)
	// 1 point, max = 2, percentage = (1*100)/2 = 50
	if report.Percentage != 50 {
		t.Errorf("expected percentage 50 for single warning, got %d", report.Percentage)
	}
}

func TestCalculateOverallScore_LargeSet(t *testing.T) {
	c := &Checker{}
	checks := make([]ComplianceCheck, 10)
	for i := range checks {
		checks[i] = ComplianceCheck{ID: "c", Status: "passed"}
	}
	// Set 2 to warning and 1 to failed.
	checks[3].Status = "warning"
	checks[7].Status = "warning"
	checks[9].Status = "failed"

	report := &ComplianceReport{Checks: checks}
	c.calculateOverallScore(report)
	// 7 passed * 2 + 2 warning * 1 + 1 failed * 0 = 16, max = 20, percentage = (16*100)/20 = 80
	if report.Percentage != 80 {
		t.Errorf("expected percentage 80, got %d", report.Percentage)
	}
}

func TestComplianceReport_Structure(t *testing.T) {
	report := ComplianceReport{
		OverallScore:    17,
		MaxScore:        20,
		Percentage:      85,
		MFAAdoption:     92.5,
		EncryptionRate:  100.0,
		PostureRate:     87.5,
		AuditLogEnabled: true,
		PasswordPolicy:  true,
		SessionTimeout:  true,
		Checks: []ComplianceCheck{
			{
				ID:          "soc2-cc6.1-mfa",
				Framework:   "SOC2",
				Name:        "Multi-Factor Authentication",
				Description: "All users should have MFA enabled",
				Status:      "warning",
				Details:     "92.5% of users have MFA enabled",
			},
		},
	}

	if report.Percentage != 85 {
		t.Errorf("Percentage = %d, want 85", report.Percentage)
	}
	if report.MFAAdoption != 92.5 {
		t.Errorf("MFAAdoption = %f, want 92.5", report.MFAAdoption)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}
	if report.Checks[0].Framework != "SOC2" {
		t.Errorf("Framework = %q, want SOC2", report.Checks[0].Framework)
	}
	if report.Checks[0].Status != "warning" {
		t.Errorf("Status = %q, want warning", report.Checks[0].Status)
	}
}

func TestComplianceCheck_Statuses(t *testing.T) {
	validStatuses := []string{"passed", "failed", "warning"}
	for _, status := range validStatuses {
		check := ComplianceCheck{
			ID:     "test",
			Status: status,
		}
		if check.Status != status {
			t.Errorf("Status = %q, want %q", check.Status, status)
		}
	}
}
