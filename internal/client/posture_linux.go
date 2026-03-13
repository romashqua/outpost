//go:build linux

package client

import (
	"os"
	"os/exec"
	"strings"
)

func collectPlatformPosture(p *DevicePosture) {
	p.DiskEncrypted = checkLinuxLUKS()
	p.FirewallEnabled = checkLinuxFirewall()
	p.ScreenLockEnabled = true // Assume enabled on Linux.
	p.OSVersion = getLinuxVersion()
}

func checkLinuxLUKS() bool {
	out, err := exec.Command("lsblk", "-o", "TYPE").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "crypt")
}

func checkLinuxFirewall() bool {
	// Check iptables.
	if out, err := exec.Command("iptables", "-L", "-n").Output(); err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) > 10 {
			return true
		}
	}

	// Check nftables.
	if out, err := exec.Command("nft", "list", "ruleset").Output(); err == nil {
		if len(strings.TrimSpace(string(out))) > 0 {
			return true
		}
	}

	// Check ufw.
	if out, err := exec.Command("ufw", "status").Output(); err == nil {
		return strings.Contains(string(out), "active")
	}

	return false
}

func getLinuxVersion() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION_ID=") {
			return strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}
	return "unknown"
}
