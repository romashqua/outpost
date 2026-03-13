package ztna

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DevicePosture represents the security state of a client device.
type DevicePosture struct {
	DeviceID          string
	OSType            string // windows, macos, linux, ios, android
	OSVersion         string
	DiskEncrypted     bool
	ScreenLockEnabled bool
	AntivirusActive   bool
	FirewallEnabled   bool
	LastChecked       time.Time
	Score             int // 0-100 compliance score
}

// PosturePolicy defines what device state is required to access a network.
type PosturePolicy struct {
	ID                    string
	Name                  string
	NetworkID             string
	RequireDiskEncryption bool
	RequireScreenLock     bool
	RequireAntivirus      bool
	RequireFirewall       bool
	MinOSVersion          map[string]string // e.g., {"windows": "10.0.19044", "macos": "13.0"}
	MinPostureScore       int
}

// PostureChecker evaluates device posture against policies.
type PostureChecker struct{}

// NewPostureChecker creates a new PostureChecker.
func NewPostureChecker() *PostureChecker {
	return &PostureChecker{}
}

// Evaluate checks a device's posture against a policy.
// Returns compliant (bool), score (int), and list of violations.
func (pc *PostureChecker) Evaluate(posture DevicePosture, policy PosturePolicy) (bool, int, []string) {
	var violations []string
	score := 100

	// Check disk encryption.
	if policy.RequireDiskEncryption && !posture.DiskEncrypted {
		violations = append(violations, "disk encryption is not enabled")
		score -= 25
	}

	// Check screen lock.
	if policy.RequireScreenLock && !posture.ScreenLockEnabled {
		violations = append(violations, "screen lock is not enabled")
		score -= 15
	}

	// Check antivirus.
	if policy.RequireAntivirus && !posture.AntivirusActive {
		violations = append(violations, "antivirus is not active")
		score -= 20
	}

	// Check firewall.
	if policy.RequireFirewall && !posture.FirewallEnabled {
		violations = append(violations, "firewall is not enabled")
		score -= 15
	}

	// Check minimum OS version.
	if minVer, ok := policy.MinOSVersion[posture.OSType]; ok {
		if !isVersionAtLeast(posture.OSVersion, minVer) {
			violations = append(violations, fmt.Sprintf(
				"%s version %s is below minimum required %s",
				posture.OSType, posture.OSVersion, minVer,
			))
			score -= 25
		}
	}

	// Clamp score to 0-100.
	if score < 0 {
		score = 0
	}

	// Check minimum posture score threshold.
	if policy.MinPostureScore > 0 && score < policy.MinPostureScore {
		violations = append(violations, fmt.Sprintf(
			"posture score %d is below minimum required %d",
			score, policy.MinPostureScore,
		))
	}

	compliant := len(violations) == 0
	return compliant, score, violations
}

// isVersionAtLeast compares two dot-separated version strings and returns
// true if actual >= required.
func isVersionAtLeast(actual, required string) bool {
	actualParts := strings.Split(actual, ".")
	requiredParts := strings.Split(required, ".")

	maxLen := len(actualParts)
	if len(requiredParts) > maxLen {
		maxLen = len(requiredParts)
	}

	for i := range maxLen {
		a := 0
		if i < len(actualParts) {
			a, _ = strconv.Atoi(actualParts[i])
		}

		r := 0
		if i < len(requiredParts) {
			r, _ = strconv.Atoi(requiredParts[i])
		}

		if a > r {
			return true
		}
		if a < r {
			return false
		}
	}

	return true // equal
}
