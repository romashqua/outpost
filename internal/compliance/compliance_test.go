package compliance

import (
	"testing"
)

func TestCalculateOverallScore_AllPass(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "pass"},
			{ID: "c2", Status: "pass"},
			{ID: "c3", Status: "pass"},
		},
	}

	score := c.calculateOverallScore(report)
	// 3 checks * 2 points = 6, max = 6, score = (6*100)/6 = 100
	if score != 100 {
		t.Errorf("expected score 100 for all pass, got %d", score)
	}
}

func TestCalculateOverallScore_AllFail(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "fail"},
			{ID: "c2", Status: "fail"},
			{ID: "c3", Status: "fail"},
		},
	}

	score := c.calculateOverallScore(report)
	// 0 points, max = 6, score = 0
	if score != 0 {
		t.Errorf("expected score 0 for all fail, got %d", score)
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

	score := c.calculateOverallScore(report)
	// 4 warnings * 1 point = 4, max = 8, score = (4*100)/8 = 50
	if score != 50 {
		t.Errorf("expected score 50 for all warning, got %d", score)
	}
}

func TestCalculateOverallScore_Mixed(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "pass"},    // 2 points
			{ID: "c2", Status: "warning"}, // 1 point
			{ID: "c3", Status: "fail"},    // 0 points
			{ID: "c4", Status: "pass"},    // 2 points
		},
	}

	score := c.calculateOverallScore(report)
	// 5 points, max = 8, score = (5*100)/8 = 62
	if score != 62 {
		t.Errorf("expected score 62 for mixed statuses, got %d", score)
	}
}

func TestCalculateOverallScore_NoChecks(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{},
	}

	score := c.calculateOverallScore(report)
	if score != 0 {
		t.Errorf("expected score 0 for no checks, got %d", score)
	}
}

func TestCalculateOverallScore_SinglePass(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "pass"},
		},
	}

	score := c.calculateOverallScore(report)
	// 2 points, max = 2, score = 100
	if score != 100 {
		t.Errorf("expected score 100 for single pass, got %d", score)
	}
}

func TestCalculateOverallScore_SingleFail(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "fail"},
		},
	}

	score := c.calculateOverallScore(report)
	if score != 0 {
		t.Errorf("expected score 0 for single fail, got %d", score)
	}
}

func TestCalculateOverallScore_SingleWarning(t *testing.T) {
	c := &Checker{}
	report := &ComplianceReport{
		Checks: []ComplianceCheck{
			{ID: "c1", Status: "warning"},
		},
	}

	score := c.calculateOverallScore(report)
	// 1 point, max = 2, score = (1*100)/2 = 50
	if score != 50 {
		t.Errorf("expected score 50 for single warning, got %d", score)
	}
}

func TestCalculateOverallScore_LargeSet(t *testing.T) {
	c := &Checker{}
	checks := make([]ComplianceCheck, 10)
	for i := range checks {
		checks[i] = ComplianceCheck{ID: "c", Status: "pass"}
	}
	// Set 2 to warning and 1 to fail.
	checks[3].Status = "warning"
	checks[7].Status = "warning"
	checks[9].Status = "fail"

	report := &ComplianceReport{Checks: checks}
	score := c.calculateOverallScore(report)
	// 7 pass * 2 + 2 warning * 1 + 1 fail * 0 = 16, max = 20, score = (16*100)/20 = 80
	if score != 80 {
		t.Errorf("expected score 80, got %d", score)
	}
}

func TestComplianceReport_Structure(t *testing.T) {
	report := ComplianceReport{
		OverallScore:    85,
		MFAAdoption:     92.5,
		EncryptionRate:  100.0,
		PostureRate:     87.5,
		AuditLogEnabled: true,
		PasswordPolicy:  true,
		SessionTimeout:  true,
		Checks: []ComplianceCheck{
			{
				ID:          "soc2-cc6.1-mfa",
				Category:    "SOC2",
				Name:        "Multi-Factor Authentication",
				Description: "All users should have MFA enabled",
				Status:      "warning",
				Details:     "92.5% of users have MFA enabled",
			},
		},
	}

	if report.OverallScore != 85 {
		t.Errorf("OverallScore = %d, want 85", report.OverallScore)
	}
	if report.MFAAdoption != 92.5 {
		t.Errorf("MFAAdoption = %f, want 92.5", report.MFAAdoption)
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(report.Checks))
	}
	if report.Checks[0].Category != "SOC2" {
		t.Errorf("Category = %q, want SOC2", report.Checks[0].Category)
	}
	if report.Checks[0].Status != "warning" {
		t.Errorf("Status = %q, want warning", report.Checks[0].Status)
	}
}

func TestComplianceCheck_Statuses(t *testing.T) {
	validStatuses := []string{"pass", "fail", "warning"}
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
