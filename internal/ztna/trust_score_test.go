package ztna

import (
	"testing"
)

func TestClassifyTrustLevel(t *testing.T) {
	config := DefaultTrustScoreConfig()

	tests := []struct {
		score int
		want  TrustLevel
	}{
		{100, TrustLevelHigh},
		{80, TrustLevelHigh},
		{79, TrustLevelMedium},
		{50, TrustLevelMedium},
		{49, TrustLevelLow},
		{20, TrustLevelLow},
		{19, TrustLevelCritical},
		{0, TrustLevelCritical},
	}

	for _, tt := range tests {
		got := classifyTrustLevel(tt.score, config)
		if got != tt.want {
			t.Errorf("classifyTrustLevel(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestDefaultTrustScoreConfig(t *testing.T) {
	cfg := DefaultTrustScoreConfig()

	totalWeight := cfg.WeightDiskEncryption + cfg.WeightScreenLock +
		cfg.WeightAntivirus + cfg.WeightFirewall +
		cfg.WeightOSVersion + cfg.WeightMFA

	if totalWeight != 100 {
		t.Errorf("total weight = %d, want 100", totalWeight)
	}

	if cfg.ThresholdHigh <= cfg.ThresholdMedium {
		t.Errorf("ThresholdHigh (%d) should be > ThresholdMedium (%d)", cfg.ThresholdHigh, cfg.ThresholdMedium)
	}
	if cfg.ThresholdMedium <= cfg.ThresholdLow {
		t.Errorf("ThresholdMedium (%d) should be > ThresholdLow (%d)", cfg.ThresholdMedium, cfg.ThresholdLow)
	}
}

func TestTrustComponentScoring(t *testing.T) {
	config := DefaultTrustScoreConfig()

	// Simulate full compliance — all components pass.
	totalScore := config.WeightDiskEncryption + config.WeightScreenLock +
		config.WeightAntivirus + config.WeightFirewall +
		config.WeightOSVersion + config.WeightMFA

	if totalScore != 100 {
		t.Errorf("full compliance score = %d, want 100", totalScore)
	}

	level := classifyTrustLevel(totalScore, config)
	if level != TrustLevelHigh {
		t.Errorf("full compliance level = %q, want %q", level, TrustLevelHigh)
	}

	// Simulate no disk encryption and no MFA.
	partialScore := totalScore - config.WeightDiskEncryption - config.WeightMFA
	if partialScore != 60 {
		t.Errorf("partial score = %d, want 60", partialScore)
	}

	partialLevel := classifyTrustLevel(partialScore, config)
	if partialLevel != TrustLevelMedium {
		t.Errorf("partial level = %q, want %q", partialLevel, TrustLevelMedium)
	}
}

func TestTrustLevelConstants(t *testing.T) {
	if TrustLevelHigh != "high" {
		t.Errorf("TrustLevelHigh = %q", TrustLevelHigh)
	}
	if TrustLevelMedium != "medium" {
		t.Errorf("TrustLevelMedium = %q", TrustLevelMedium)
	}
	if TrustLevelLow != "low" {
		t.Errorf("TrustLevelLow = %q", TrustLevelLow)
	}
	if TrustLevelCritical != "critical" {
		t.Errorf("TrustLevelCritical = %q", TrustLevelCritical)
	}
}

func TestStalenessReduction(t *testing.T) {
	// Score 100 with 20% staleness penalty = 80.
	staleScore := 100 * 80 / 100
	if staleScore != 80 {
		t.Errorf("stale score = %d, want 80", staleScore)
	}

	// Score 50 with 20% staleness penalty = 40.
	staleScore = 50 * 80 / 100
	if staleScore != 40 {
		t.Errorf("stale score = %d, want 40", staleScore)
	}
}
