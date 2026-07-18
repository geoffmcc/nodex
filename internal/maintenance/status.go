// Package maintenance interprets read-only preflight results and builds
// immutable, tamper-evident maintenance plans. It never executes anything:
// execution belongs to the Ansible adapter (invoked by the CLI layer for
// the read-only check-updates/verify-host operations) and, in a later
// phase, to `maintenance apply`.
package maintenance

import (
	"strings"

	"github.com/geoffmcc/nodex/internal/ansible"
)

// Task names from the embedded check-updates/verify-host playbooks. These
// are the join points between playbook content and interpretation; the
// playbooks are embedded in the same binary, so they cannot drift apart at
// runtime.
const (
	taskUpgradable     = "List upgradable packages"
	taskUpgradeSim     = "Simulate dist-upgrade"
	taskRebootRequired = "Check reboot-required marker"
	taskFailedUnits    = "List failed systemd units"
	taskRootUsage      = "Report root filesystem usage"
	taskDebianAssert   = "Verify Debian family"
)

// HostStatus is the interpreted preflight state of one host.
type HostStatus struct {
	Host            string   `json:"host" yaml:"host"`
	Reachable       bool     `json:"reachable" yaml:"reachable"`
	Supported       bool     `json:"supported" yaml:"supported"`
	PendingUpdates  []string `json:"pending_updates,omitempty" yaml:"pending_updates,omitempty"`
	SecurityUpdates []string `json:"security_updates,omitempty" yaml:"security_updates,omitempty"`
	RebootRequired  bool     `json:"reboot_required" yaml:"reboot_required"`
	FailedUnits     []string `json:"failed_units,omitempty" yaml:"failed_units,omitempty"`
	RootUsage       string   `json:"root_usage,omitempty" yaml:"root_usage,omitempty"`
	Warnings        []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// InterpretCheckUpdates converts an adapter run of the check-updates
// operation into per-host statuses. Hosts that failed or were unreachable
// are reported as such, never silently dropped.
func InterpretCheckUpdates(res *ansible.RunResult) []HostStatus {
	statuses := make([]HostStatus, 0, len(res.Hosts))
	for _, hr := range res.Hosts {
		hs := HostStatus{
			Host:      hr.Host,
			Reachable: hr.Unreachable == 0,
			Supported: true,
		}
		if hr.Failures > 0 {
			hs.Warnings = append(hs.Warnings, "one or more preflight tasks failed")
		}
		if !hs.Reachable {
			hs.Supported = false
			hs.Warnings = append(hs.Warnings, "host unreachable")
			statuses = append(statuses, hs)
			continue
		}
		for _, outcome := range res.TaskOutcomes[hr.Host] {
			interpretOutcome(&hs, outcome)
		}
		statuses = append(statuses, hs)
	}
	return statuses
}

func interpretOutcome(hs *HostStatus, o ansible.TaskOutcome) {
	switch o.Task {
	case taskDebianAssert:
		if o.Failed {
			hs.Supported = false
			hs.Warnings = append(hs.Warnings, "unsupported distribution (Debian/Ubuntu required)")
		}
	case taskUpgradable:
		hs.PendingUpdates = parseUpgradable(o.StdoutLines)
	case taskUpgradeSim:
		hs.SecurityUpdates = parseSecurityUpdates(o.StdoutLines)
	case taskRebootRequired:
		if o.StatExists != nil {
			hs.RebootRequired = *o.StatExists
		}
	case taskFailedUnits:
		for _, line := range o.StdoutLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if fields := strings.Fields(line); len(fields) > 0 {
				hs.FailedUnits = append(hs.FailedUnits, fields[0])
			}
		}
	case taskRootUsage:
		hs.RootUsage = parseRootUsage(o.StdoutLines)
	}
}

// parseUpgradable extracts package names from `apt list --upgradable`
// output lines of the form "name/suite version arch [upgradable from: v]".
func parseUpgradable(lines []string) []string {
	var pkgs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") || strings.HasPrefix(line, "WARNING") {
			continue
		}
		if idx := strings.IndexByte(line, '/'); idx > 0 {
			pkgs = append(pkgs, line[:idx])
		}
	}
	return pkgs
}

// parseSecurityUpdates extracts package names from `apt-get -s dist-upgrade`
// simulation lines of the form "Inst name [old] (new suite ...)" whose
// source suite contains "-security".
func parseSecurityUpdates(lines []string) []string {
	var pkgs []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Inst ") || !strings.Contains(line, "-security") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			pkgs = append(pkgs, fields[1])
		}
	}
	return pkgs
}

// parseRootUsage extracts the use% of / from `df -P /` output.
func parseRootUsage(lines []string) string {
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 6 && fields[5] == "/" {
			return fields[4]
		}
	}
	return ""
}
