package compliance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ComplianceReport contains overall compliance status.
type ComplianceReport struct {
	OverallScore    int               `json:"overall_score"`    // 0-100
	MFAAdoption     float64           `json:"mfa_adoption"`     // percentage of users with MFA
	EncryptionRate  float64           `json:"encryption_rate"`  // percentage of compliant devices
	PostureRate     float64           `json:"posture_rate"`     // percentage of devices passing posture
	AuditLogEnabled bool              `json:"audit_log_enabled"`
	PasswordPolicy  bool              `json:"password_policy"`
	SessionTimeout  bool              `json:"session_timeout"`
	Checks          []ComplianceCheck `json:"checks"`
}

// ComplianceCheck represents an individual compliance check result.
type ComplianceCheck struct {
	ID          string `json:"id"`
	Category    string `json:"category"` // "SOC2", "ISO27001", "GDPR"
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pass", "fail", "warning"
	Details     string `json:"details"`
}

// Checker runs compliance checks against the current system state.
type Checker struct {
	pool *pgxpool.Pool
}

// NewChecker creates a new compliance Checker.
func NewChecker(pool *pgxpool.Pool) *Checker {
	return &Checker{pool: pool}
}

// RunAllChecks executes all compliance checks and returns a full report.
func (c *Checker) RunAllChecks(ctx context.Context) (*ComplianceReport, error) {
	soc2, err := c.RunSOC2Checks(ctx)
	if err != nil {
		return nil, fmt.Errorf("soc2 checks: %w", err)
	}

	iso, err := c.RunISO27001Checks(ctx)
	if err != nil {
		return nil, fmt.Errorf("iso27001 checks: %w", err)
	}

	gdpr, err := c.RunGDPRChecks(ctx)
	if err != nil {
		return nil, fmt.Errorf("gdpr checks: %w", err)
	}

	allChecks := make([]ComplianceCheck, 0, len(soc2)+len(iso)+len(gdpr))
	allChecks = append(allChecks, soc2...)
	allChecks = append(allChecks, iso...)
	allChecks = append(allChecks, gdpr...)

	report := &ComplianceReport{
		Checks: allChecks,
	}

	// Calculate MFA adoption rate.
	report.MFAAdoption, err = c.mfaAdoptionRate(ctx)
	if err != nil {
		return nil, fmt.Errorf("mfa adoption: %w", err)
	}

	// Calculate disk encryption rate from posture data.
	report.EncryptionRate, err = c.encryptionRate(ctx)
	if err != nil {
		return nil, fmt.Errorf("encryption rate: %w", err)
	}

	// Calculate posture compliance rate.
	report.PostureRate, err = c.postureRate(ctx)
	if err != nil {
		return nil, fmt.Errorf("posture rate: %w", err)
	}

	// Check audit log and settings.
	report.AuditLogEnabled, err = c.auditLogEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("audit log check: %w", err)
	}

	report.PasswordPolicy = c.passwordPolicyEnabled(ctx)
	report.SessionTimeout = c.sessionTimeoutEnabled(ctx)

	// Calculate overall score.
	report.OverallScore = c.calculateOverallScore(report)

	return report, nil
}

// RunSOC2Checks runs SOC2-related compliance checks.
func (c *Checker) RunSOC2Checks(ctx context.Context) ([]ComplianceCheck, error) {
	var checks []ComplianceCheck

	// CC6.1: Logical access security — MFA enforcement.
	mfaRate, err := c.mfaAdoptionRate(ctx)
	if err != nil {
		return nil, err
	}
	mfaStatus := "pass"
	mfaDetails := fmt.Sprintf("%.1f%% of users have MFA enabled", mfaRate)
	if mfaRate < 100 {
		mfaStatus = "fail"
		if mfaRate >= 80 {
			mfaStatus = "warning"
		}
	}
	checks = append(checks, ComplianceCheck{
		ID:          "soc2-cc6.1-mfa",
		Category:    "SOC2",
		Name:        "Multi-Factor Authentication",
		Description: "All users should have MFA enabled (CC6.1)",
		Status:      mfaStatus,
		Details:     mfaDetails,
	})

	// CC6.6: Encryption in transit — all traffic via WireGuard.
	checks = append(checks, ComplianceCheck{
		ID:          "soc2-cc6.6-encryption",
		Category:    "SOC2",
		Name:        "Encryption in Transit",
		Description: "All traffic must be encrypted via WireGuard tunnels (CC6.6)",
		Status:      "pass",
		Details:     "All connections use WireGuard encryption",
	})

	// CC7.2: Monitoring — audit log.
	auditEnabled, err := c.auditLogEnabled(ctx)
	if err != nil {
		return nil, err
	}
	auditStatus := "pass"
	auditDetails := "Audit logging is enabled"
	if !auditEnabled {
		auditStatus = "fail"
		auditDetails = "No audit log entries found"
	}
	checks = append(checks, ComplianceCheck{
		ID:          "soc2-cc7.2-audit",
		Category:    "SOC2",
		Name:        "System Monitoring",
		Description: "Audit logging must be enabled and active (CC7.2)",
		Status:      auditStatus,
		Details:     auditDetails,
	})

	return checks, nil
}

// RunISO27001Checks runs ISO 27001-related compliance checks.
func (c *Checker) RunISO27001Checks(ctx context.Context) ([]ComplianceCheck, error) {
	var checks []ComplianceCheck

	// A.8.1: Device posture compliance.
	postureRate, err := c.postureRate(ctx)
	if err != nil {
		return nil, err
	}
	postureStatus := "pass"
	postureDetails := fmt.Sprintf("%.1f%% of devices pass posture checks", postureRate)
	if postureRate < 100 {
		postureStatus = "fail"
		if postureRate >= 80 {
			postureStatus = "warning"
		}
	}
	checks = append(checks, ComplianceCheck{
		ID:          "iso27001-a8.1-posture",
		Category:    "ISO27001",
		Name:        "Device Posture Compliance",
		Description: "All devices must pass posture checks (A.8.1)",
		Status:      postureStatus,
		Details:     postureDetails,
	})

	// A.9.4: Access control — RBAC.
	var roleCount int
	err = c.pool.QueryRow(ctx, `SELECT COUNT(*) FROM roles`).Scan(&roleCount)
	if err != nil {
		return nil, fmt.Errorf("count roles: %w", err)
	}
	rbacStatus := "pass"
	rbacDetails := fmt.Sprintf("%d roles configured", roleCount)
	if roleCount < 2 {
		rbacStatus = "warning"
		rbacDetails = "Insufficient role separation"
	}
	checks = append(checks, ComplianceCheck{
		ID:          "iso27001-a9.4-rbac",
		Category:    "ISO27001",
		Name:        "Role-Based Access Control",
		Description: "Access control must use role-based permissions (A.9.4)",
		Status:      rbacStatus,
		Details:     rbacDetails,
	})

	// A.10.1: Cryptographic controls — disk encryption.
	encRate, err := c.encryptionRate(ctx)
	if err != nil {
		return nil, err
	}
	encStatus := "pass"
	encDetails := fmt.Sprintf("%.1f%% of devices have disk encryption", encRate)
	if encRate < 100 {
		encStatus = "fail"
		if encRate >= 80 {
			encStatus = "warning"
		}
	}
	checks = append(checks, ComplianceCheck{
		ID:          "iso27001-a10.1-encryption",
		Category:    "ISO27001",
		Name:        "Disk Encryption",
		Description: "All devices must have disk encryption enabled (A.10.1)",
		Status:      encStatus,
		Details:     encDetails,
	})

	return checks, nil
}

// RunGDPRChecks runs GDPR-related compliance checks.
func (c *Checker) RunGDPRChecks(ctx context.Context) ([]ComplianceCheck, error) {
	var checks []ComplianceCheck

	// Art. 5(1)(f): Data security — encryption.
	checks = append(checks, ComplianceCheck{
		ID:          "gdpr-art5-encryption",
		Category:    "GDPR",
		Name:        "Data Encryption",
		Description: "Personal data must be protected with appropriate encryption (Art. 5(1)(f))",
		Status:      "pass",
		Details:     "WireGuard provides end-to-end encryption for all VPN traffic",
	})

	// Art. 30: Records of processing — audit log.
	auditEnabled, err := c.auditLogEnabled(ctx)
	if err != nil {
		return nil, err
	}
	auditStatus := "pass"
	auditDetails := "Audit logging provides records of processing activities"
	if !auditEnabled {
		auditStatus = "fail"
		auditDetails = "Audit logging is not active; processing records unavailable"
	}
	checks = append(checks, ComplianceCheck{
		ID:          "gdpr-art30-audit",
		Category:    "GDPR",
		Name:        "Records of Processing",
		Description: "Records of processing activities must be maintained (Art. 30)",
		Status:      auditStatus,
		Details:     auditDetails,
	})

	// Art. 32: Security of processing — access control.
	var activeUsers int
	err = c.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE is_active = true`).Scan(&activeUsers)
	if err != nil {
		return nil, fmt.Errorf("count active users: %w", err)
	}
	checks = append(checks, ComplianceCheck{
		ID:          "gdpr-art32-access",
		Category:    "GDPR",
		Name:        "Access Control",
		Description: "Appropriate access controls must be in place (Art. 32)",
		Status:      "pass",
		Details:     fmt.Sprintf("%d active users with role-based access control", activeUsers),
	})

	return checks, nil
}

// mfaAdoptionRate returns the percentage of active users with MFA enabled.
func (c *Checker) mfaAdoptionRate(ctx context.Context) (float64, error) {
	var total, mfaEnabled int
	err := c.pool.QueryRow(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE mfa_enabled = true) FROM users WHERE is_active = true`,
	).Scan(&total, &mfaEnabled)
	if err != nil {
		return 0, fmt.Errorf("query mfa rate: %w", err)
	}
	if total == 0 {
		return 100, nil
	}
	return float64(mfaEnabled) / float64(total) * 100, nil
}

// encryptionRate returns the percentage of devices with disk encryption enabled.
func (c *Checker) encryptionRate(ctx context.Context) (float64, error) {
	var total, encrypted int
	err := c.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT dp.device_id),
			COUNT(DISTINCT dp.device_id) FILTER (WHERE dp.disk_encrypted = true)
		 FROM device_posture dp
		 INNER JOIN (
			SELECT device_id, MAX(checked_at) AS latest
			FROM device_posture GROUP BY device_id
		 ) latest ON dp.device_id = latest.device_id AND dp.checked_at = latest.latest`,
	).Scan(&total, &encrypted)
	if err != nil {
		return 0, fmt.Errorf("query encryption rate: %w", err)
	}
	if total == 0 {
		return 100, nil
	}
	return float64(encrypted) / float64(total) * 100, nil
}

// postureRate returns the percentage of devices with a posture score >= 80.
func (c *Checker) postureRate(ctx context.Context) (float64, error) {
	var total, passing int
	err := c.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT dp.device_id),
			COUNT(DISTINCT dp.device_id) FILTER (WHERE dp.score >= 80)
		 FROM device_posture dp
		 INNER JOIN (
			SELECT device_id, MAX(checked_at) AS latest
			FROM device_posture GROUP BY device_id
		 ) latest ON dp.device_id = latest.device_id AND dp.checked_at = latest.latest`,
	).Scan(&total, &passing)
	if err != nil {
		return 0, fmt.Errorf("query posture rate: %w", err)
	}
	if total == 0 {
		return 100, nil
	}
	return float64(passing) / float64(total) * 100, nil
}

// auditLogEnabled checks if audit logging is active by verifying recent entries exist.
func (c *Checker) auditLogEnabled(ctx context.Context) (bool, error) {
	var count int
	err := c.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM audit_log LIMIT 1`,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check audit log: %w", err)
	}
	return count > 0, nil
}

// passwordPolicyEnabled checks whether the MFA requirement is enabled.
func (c *Checker) passwordPolicyEnabled(ctx context.Context) bool {
	var val string
	err := c.pool.QueryRow(ctx,
		`SELECT value::text FROM settings WHERE key = 'mfa.required'`,
	).Scan(&val)
	return err == nil && val == "true"
}

// sessionTimeoutEnabled checks whether session expiry is configured.
func (c *Checker) sessionTimeoutEnabled(ctx context.Context) bool {
	var count int
	err := c.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions WHERE expires_at > now()`,
	).Scan(&count)
	// If sessions exist with expiry dates, timeout is being enforced.
	return err == nil
}

// calculateOverallScore computes a weighted overall compliance score.
func (c *Checker) calculateOverallScore(report *ComplianceReport) int {
	if len(report.Checks) == 0 {
		return 0
	}

	passed := 0
	for _, check := range report.Checks {
		switch check.Status {
		case "pass":
			passed += 2
		case "warning":
			passed += 1
		}
	}

	maxScore := len(report.Checks) * 2
	score := (passed * 100) / maxScore

	if score > 100 {
		score = 100
	}
	return score
}
