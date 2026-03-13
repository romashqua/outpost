//go:build windows

package client

import (
	"os/exec"
	"strings"
)

func collectPlatformPosture(p *DevicePosture) {
	p.DiskEncrypted = checkWindowsBitLocker()
	p.FirewallEnabled = checkWindowsFirewall()
	p.ScreenLockEnabled = checkWindowsScreenLock()
	p.OSVersion = getWindowsVersion()
}

func checkWindowsBitLocker() bool {
	out, err := exec.Command("powershell", "-Command",
		"(Get-BitLockerVolume -MountPoint 'C:').ProtectionStatus").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "On"
}

func checkWindowsFirewall() bool {
	out, err := exec.Command("powershell", "-Command",
		"(Get-NetFirewallProfile -Profile Domain,Public,Private).Enabled").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "True")
}

func checkWindowsScreenLock() bool {
	out, err := exec.Command("powershell", "-Command",
		"(Get-ItemProperty 'HKLM:\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Policies\\System' -Name 'InactivityTimeoutSecs' -ErrorAction SilentlyContinue).InactivityTimeoutSecs").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != "" && strings.TrimSpace(string(out)) != "0"
}

func getWindowsVersion() string {
	out, err := exec.Command("cmd", "/c", "ver").Output()
	if err != nil {
		return "unknown"
	}
	s := strings.TrimSpace(string(out))
	// Extract version from "Microsoft Windows [Version 10.0.22621.1234]"
	if idx := strings.Index(s, "Version "); idx >= 0 {
		s = s[idx+8:]
		if end := strings.Index(s, "]"); end >= 0 {
			return s[:end]
		}
	}
	return s
}
