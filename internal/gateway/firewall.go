package gateway

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	commonv1 "github.com/romashqua/outpost/pkg/pb/outpost/common/v1"
)

const (
	outpostChain = "OUTPOST-FWD"
	iptablesCmd  = "iptables"
)

// FirewallManager applies iptables rules based on ACL configuration
// received from core via gRPC.
type FirewallManager struct {
	logger *slog.Logger
}

// NewFirewallManager creates a new firewall manager.
func NewFirewallManager(logger *slog.Logger) *FirewallManager {
	return &FirewallManager{logger: logger}
}

// Init creates the OUTPOST-FWD chain and hooks it into FORWARD.
// Safe to call multiple times — uses -N (which fails silently if exists).
func (fm *FirewallManager) Init() error {
	// Create chain (ignore error if already exists).
	_ = fm.run("-N", outpostChain)

	// Check if FORWARD already jumps to our chain.
	if err := fm.run("-C", "FORWARD", "-j", outpostChain); err != nil {
		// Not present — insert at the top of FORWARD.
		if err := fm.run("-I", "FORWARD", "1", "-j", outpostChain); err != nil {
			return fmt.Errorf("insert OUTPOST-FWD into FORWARD: %w", err)
		}
	}

	fm.logger.Info("firewall chain initialized", "chain", outpostChain)
	return nil
}

// Apply flushes the OUTPOST-FWD chain and replaces it with the given rules.
func (fm *FirewallManager) Apply(config *commonv1.FirewallConfig) error {
	if config == nil {
		return nil
	}

	// Flush existing rules in our chain.
	if err := fm.run("-F", outpostChain); err != nil {
		return fmt.Errorf("flush chain %s: %w", outpostChain, err)
	}

	// Allow established/related connections first (return traffic for accepted sessions).
	if err := fm.run("-A", outpostChain, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		fm.logger.Warn("failed to add conntrack rule", "error", err)
	}

	applied := 0
	for _, rule := range config.GetRules() {
		args := fm.buildRuleArgs(rule)
		if args == nil {
			continue
		}
		if err := fm.run(args...); err != nil {
			fm.logger.Warn("failed to apply firewall rule",
				"source", rule.GetSource(),
				"destination", rule.GetDestination(),
				"action", rule.GetAction().String(),
				"error", err)
			continue
		}
		applied++
	}

	// Apply NAT masquerade so destination hosts can route replies back to VPN clients.
	if config.GetNatEnabled() {
		natIface := config.GetNatInterface()
		if natIface == "" {
			natIface = detectDefaultInterface()
		}
		if natIface != "" {
			if err := fm.run("-t", "nat", "-C", "POSTROUTING", "-o", natIface, "-j", "MASQUERADE"); err != nil {
				_ = fm.run("-t", "nat", "-A", "POSTROUTING", "-o", natIface, "-j", "MASQUERADE")
			}
			fm.logger.Info("NAT masquerade configured", "interface", natIface)
		} else {
			fm.logger.Warn("NAT enabled but no outbound interface detected — masquerade skipped")
		}
	}

	// Flush conntrack entries for source IPs that now have DROP rules.
	// This ensures existing connections are torn down immediately instead of
	// persisting through the ESTABLISHED,RELATED rule.
	fm.flushConntrackForDroppedSources(config)

	fm.logger.Info("firewall rules applied", "total", len(config.GetRules()), "applied", applied)
	return nil
}

// buildRuleArgs converts a FirewallRule proto to iptables arguments.
func (fm *FirewallManager) buildRuleArgs(rule *commonv1.FirewallRule) []string {
	if rule.GetSource() == "" {
		return nil
	}

	args := []string{"-A", outpostChain}

	args = append(args, "-s", rule.GetSource())

	if rule.GetDestination() != "" {
		args = append(args, "-d", rule.GetDestination())
	}

	if rule.GetProtocol() != "" {
		args = append(args, "-p", rule.GetProtocol())
		if rule.GetPort() != "" {
			args = append(args, "--dport", rule.GetPort())
		}
	}

	switch rule.GetAction() {
	case commonv1.FirewallRule_ACTION_ACCEPT:
		args = append(args, "-j", "ACCEPT")
	case commonv1.FirewallRule_ACTION_DROP:
		args = append(args, "-j", "DROP")
	default:
		return nil
	}

	return args
}

// detectDefaultInterface returns the network interface used for the default route
// by parsing `ip route show default`. Returns empty string if detection fails.
func detectDefaultInterface() string {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}
	// Output format: "default via 172.17.0.1 dev eth0 ..."
	fields := strings.Fields(strings.TrimSpace(string(out)))
	for i, f := range fields {
		if f == "dev" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// flushConntrackForDroppedSources removes conntrack entries for source IPs
// that have DROP rules, so existing connections are killed immediately.
func (fm *FirewallManager) flushConntrackForDroppedSources(config *commonv1.FirewallConfig) {
	seen := make(map[string]bool)
	for _, rule := range config.GetRules() {
		if rule.GetAction() != commonv1.FirewallRule_ACTION_DROP {
			continue
		}
		src := rule.GetSource()
		if src == "" || seen[src] {
			continue
		}
		seen[src] = true
		// Strip CIDR suffix for conntrack (e.g. "10.10.0.2/32" → "10.10.0.2").
		ip := strings.Split(src, "/")[0]
		cmd := exec.Command("conntrack", "-D", "-s", ip)
		if out, err := cmd.CombinedOutput(); err != nil {
			// conntrack -D returns error if no entries found — that's fine.
			fm.logger.Debug("conntrack flush", "ip", ip, "output", strings.TrimSpace(string(out)))
		} else {
			fm.logger.Info("flushed conntrack entries", "ip", ip)
		}
	}
}

// run executes an iptables command.
func (fm *FirewallManager) run(args ...string) error {
	cmd := exec.Command(iptablesCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

const smartChain = "OUTPOST-SMART"

// ApplySmartRoutes creates and populates the OUTPOST-SMART chain with CIDR-based
// block/direct rules. Domain-based entries are logged as unsupported.
func (fm *FirewallManager) ApplySmartRoutes(config *commonv1.SmartRouteConfig) error {
	if config == nil || len(config.GetRules()) == 0 {
		// Remove smart chain if it exists and no rules.
		_ = fm.run("-D", "FORWARD", "-j", smartChain)
		_ = fm.run("-F", smartChain)
		_ = fm.run("-X", smartChain)
		return nil
	}

	// Create chain (ignore error if already exists).
	_ = fm.run("-N", smartChain)

	// Flush existing rules.
	if err := fm.run("-F", smartChain); err != nil {
		return fmt.Errorf("flush chain %s: %w", smartChain, err)
	}

	// Ensure OUTPOST-SMART is inserted before OUTPOST-FWD in FORWARD.
	if err := fm.run("-C", "FORWARD", "-j", smartChain); err != nil {
		if err := fm.run("-I", "FORWARD", "1", "-j", smartChain); err != nil {
			return fmt.Errorf("insert %s into FORWARD: %w", smartChain, err)
		}
	}

	applied := 0
	for _, rule := range config.GetRules() {
		switch rule.GetEntryType() {
		case "cidr":
			var action string
			switch rule.GetAction() {
			case "block":
				action = "DROP"
			case "direct":
				action = "ACCEPT"
			default:
				fm.logger.Debug("smart route: unsupported action for cidr", "action", rule.GetAction(), "value", rule.GetValue())
				continue
			}
			if err := fm.run("-A", smartChain, "-d", rule.GetValue(), "-j", action); err != nil {
				fm.logger.Warn("failed to apply smart route rule", "cidr", rule.GetValue(), "action", action, "error", err)
				continue
			}
			applied++
		case "domain", "domain_suffix":
			fm.logger.Debug("smart route: domain-based routing not yet supported", "type", rule.GetEntryType(), "value", rule.GetValue())
		}
	}

	fm.logger.Info("smart route rules applied", "total", len(config.GetRules()), "applied", applied)
	return nil
}

// Cleanup removes the OUTPOST-FWD and OUTPOST-SMART chains and their FORWARD references.
func (fm *FirewallManager) Cleanup() {
	_ = fm.run("-D", "FORWARD", "-j", smartChain)
	_ = fm.run("-F", smartChain)
	_ = fm.run("-X", smartChain)
	_ = fm.run("-D", "FORWARD", "-j", outpostChain)
	_ = fm.run("-F", outpostChain)
	_ = fm.run("-X", outpostChain)
	fm.logger.Info("firewall chains cleaned up")
}
