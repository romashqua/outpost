//go:build darwin

package client

import (
	"os/exec"
	"strings"
)

func collectPlatformPosture(p *DevicePosture) {
	p.DiskEncrypted = checkMacOSFileVault()
	p.FirewallEnabled = checkMacOSFirewall()
	p.ScreenLockEnabled = checkMacOSScreenLock()
	p.OSVersion = getMacOSVersion()
}

func checkMacOSFileVault() bool {
	out, err := exec.Command("fdesetup", "status").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "FileVault is On")
}

func checkMacOSFirewall() bool {
	out, err := exec.Command("/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "enabled")
}

func checkMacOSScreenLock() bool {
	out, err := exec.Command("sysadminctl", "-screenLock", "status").Output()
	if err != nil {
		// Fallback: check if screen saver password is required.
		out2, err2 := exec.Command("defaults", "read", "com.apple.screensaver", "askForPassword").Output()
		if err2 != nil {
			return false
		}
		return strings.TrimSpace(string(out2)) == "1"
	}
	return strings.Contains(string(out), "screenLock is on")
}

func getMacOSVersion() string {
	out, err := exec.Command("sw_vers", "-productVersion").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}
