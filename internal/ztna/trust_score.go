package ztna

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TrustLevel categorizes device trust into actionable tiers.
type TrustLevel string

const (
	TrustLevelHigh     TrustLevel = "high"     // 80-100: full access
	TrustLevelMedium   TrustLevel = "medium"   // 50-79: restricted access
	TrustLevelLow      TrustLevel = "low"      // 20-49: minimal access
	TrustLevelCritical TrustLevel = "critical"  // 0-19: blocked
)

// TrustScoreConfig defines how trust scores are calculated and acted upon.
type TrustScoreConfig struct {
	// Component weights (must sum to 100).
	WeightDiskEncryption int `json:"weight_disk_encryption"` // default 25
	WeightScreenLock     int `json:"weight_screen_lock"`     // default 10
	WeightAntivirus      int `json:"weight_antivirus"`       // default 20
	WeightFirewall       int `json:"weight_firewall"`        // default 15
	WeightOSVersion      int `json:"weight_os_version"`      // default 15
	WeightMFA            int `json:"weight_mfa"`             // default 15

	// Thresholds for trust levels.
	ThresholdHigh   int `json:"threshold_high"`   // default 80
	ThresholdMedium int `json:"threshold_medium"` // default 50
	ThresholdLow    int `json:"threshold_low"`    // default 20

	// Auto-actions.
	AutoRestrictBelowMedium bool `json:"auto_restrict_below_medium"` // restrict to safe nets
	AutoBlockBelowLow       bool `json:"auto_block_below_low"`       // remove WG peer
}

// DefaultTrustScoreConfig returns the default trust score configuration.
func DefaultTrustScoreConfig() TrustScoreConfig {
	return TrustScoreConfig{
		WeightDiskEncryption: 25,
		WeightScreenLock:     10,
		WeightAntivirus:      20,
		WeightFirewall:       15,
		WeightOSVersion:      15,
		WeightMFA:            15,
		ThresholdHigh:        80,
		ThresholdMedium:      50,
		ThresholdLow:         20,
		AutoRestrictBelowMedium: false,
		AutoBlockBelowLow:       false,
	}
}

// TrustScoreResult holds the computed trust score with breakdown.
type TrustScoreResult struct {
	DeviceID   uuid.UUID  `json:"device_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Score      int        `json:"score"`
	Level      TrustLevel `json:"level"`
	Components []TrustComponent `json:"components"`
	Violations []string   `json:"violations"`
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// TrustComponent is one factor contributing to the trust score.
type TrustComponent struct {
	Name    string `json:"name"`
	Weight  int    `json:"weight"`
	Passed  bool   `json:"passed"`
	Score   int    `json:"score"`    // points earned (0 or weight)
	Details string `json:"details"`
}

// TrustScoreCalculator computes device trust scores.
type TrustScoreCalculator struct {
	pool   *pgxpool.Pool
	config TrustScoreConfig
}

// NewTrustScoreCalculator creates a new calculator.
func NewTrustScoreCalculator(pool *pgxpool.Pool, config TrustScoreConfig) *TrustScoreCalculator {
	return &TrustScoreCalculator{pool: pool, config: config}
}

// Calculate computes the trust score for a device.
func (c *TrustScoreCalculator) Calculate(ctx context.Context, deviceID uuid.UUID) (*TrustScoreResult, error) {
	// Get latest posture data.
	var (
		userID        uuid.UUID
		osType        string
		osVersion     string
		diskEncrypted bool
		screenLock    bool
		antivirus     bool
		firewall      bool
		postureScore  int
		checkedAt     time.Time
	)

	err := c.pool.QueryRow(ctx, `
		SELECT d.user_id, dp.os_type, dp.os_version, dp.disk_encrypted,
		       dp.screen_lock, dp.antivirus, dp.firewall, dp.score, dp.checked_at
		FROM device_posture dp
		JOIN devices d ON d.id = dp.device_id
		WHERE dp.device_id = $1
		ORDER BY dp.checked_at DESC
		LIMIT 1
	`, deviceID).Scan(&userID, &osType, &osVersion, &diskEncrypted,
		&screenLock, &antivirus, &firewall, &postureScore, &checkedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No posture data — return zero score.
			return &TrustScoreResult{
				DeviceID:    deviceID,
				Score:       0,
				Level:       TrustLevelCritical,
				Violations:  []string{"no posture data available"},
				EvaluatedAt: time.Now(),
			}, nil
		}
		return nil, err
	}

	// Check if user has MFA enabled.
	var mfaEnabled bool
	err = c.pool.QueryRow(ctx, `SELECT mfa_enabled FROM users WHERE id = $1`, userID).Scan(&mfaEnabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// Calculate components.
	components := make([]TrustComponent, 0, 6)
	totalScore := 0
	var violations []string

	// Disk encryption.
	comp := TrustComponent{Name: "disk_encryption", Weight: c.config.WeightDiskEncryption, Passed: diskEncrypted}
	if diskEncrypted {
		comp.Score = c.config.WeightDiskEncryption
		comp.Details = "Full disk encryption enabled"
	} else {
		comp.Details = "Disk encryption not detected"
		violations = append(violations, "disk encryption not enabled")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// Screen lock.
	comp = TrustComponent{Name: "screen_lock", Weight: c.config.WeightScreenLock, Passed: screenLock}
	if screenLock {
		comp.Score = c.config.WeightScreenLock
		comp.Details = "Screen lock enabled"
	} else {
		comp.Details = "Screen lock not enabled"
		violations = append(violations, "screen lock not enabled")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// Antivirus.
	comp = TrustComponent{Name: "antivirus", Weight: c.config.WeightAntivirus, Passed: antivirus}
	if antivirus {
		comp.Score = c.config.WeightAntivirus
		comp.Details = "Antivirus active"
	} else {
		comp.Details = "Antivirus not detected"
		violations = append(violations, "antivirus not active")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// Firewall.
	comp = TrustComponent{Name: "firewall", Weight: c.config.WeightFirewall, Passed: firewall}
	if firewall {
		comp.Score = c.config.WeightFirewall
		comp.Details = "Firewall enabled"
	} else {
		comp.Details = "Firewall not enabled"
		violations = append(violations, "firewall not enabled")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// OS version (simplified: pass if posture score from OS check was ok).
	osOk := postureScore >= 75
	comp = TrustComponent{Name: "os_version", Weight: c.config.WeightOSVersion, Passed: osOk}
	if osOk {
		comp.Score = c.config.WeightOSVersion
		comp.Details = osType + " " + osVersion
	} else {
		comp.Details = osType + " " + osVersion + " — may need update"
		violations = append(violations, "OS version may be outdated")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// MFA.
	comp = TrustComponent{Name: "mfa", Weight: c.config.WeightMFA, Passed: mfaEnabled}
	if mfaEnabled {
		comp.Score = c.config.WeightMFA
		comp.Details = "MFA enabled for user"
	} else {
		comp.Details = "MFA not enabled"
		violations = append(violations, "MFA not enabled for device owner")
	}
	totalScore += comp.Score
	components = append(components, comp)

	// Determine trust level.
	level := classifyTrustLevel(totalScore, c.config)

	// Check posture data freshness (stale > 24h = penalty).
	if time.Since(checkedAt) > 24*time.Hour {
		totalScore = totalScore * 80 / 100 // 20% penalty for stale data
		violations = append(violations, "posture data is stale (>24h)")
		level = classifyTrustLevel(totalScore, c.config)
	}

	result := &TrustScoreResult{
		DeviceID:    deviceID,
		UserID:      userID,
		Score:       totalScore,
		Level:       level,
		Components:  components,
		Violations:  violations,
		EvaluatedAt: time.Now(),
	}

	// Store score in DB.
	_, _ = c.pool.Exec(ctx, `
		INSERT INTO device_trust_scores (device_id, score, level, violations, evaluated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, deviceID, totalScore, string(level), violations, result.EvaluatedAt)

	return result, nil
}

func classifyTrustLevel(score int, config TrustScoreConfig) TrustLevel {
	switch {
	case score >= config.ThresholdHigh:
		return TrustLevelHigh
	case score >= config.ThresholdMedium:
		return TrustLevelMedium
	case score >= config.ThresholdLow:
		return TrustLevelLow
	default:
		return TrustLevelCritical
	}
}
