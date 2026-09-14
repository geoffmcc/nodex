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

// Stable evidence IDs from the embedded check-updates/verify-host playbooks.
// Display task names are presentation text and are never interpretation keys.
const (
	taskConnectivity   = ansible.HostEvidenceConnectivity
	taskUpgradable     = ansible.HostEvidencePackages
	taskUpgradeSim     = ansible.HostEvidenceSimulation
	taskRebootRequired = ansible.HostEvidenceReboot
	taskFailedUnits    = ansible.HostEvidenceFailedUnits
	taskRootUsage      = ansible.HostEvidenceRoot
	taskDebianAssert   = ansible.HostEvidenceDebian
	taskRefresh        = ansible.HostEvidenceRefresh
)

// HostStatus is the interpreted preflight state of one host.
type HostStatus struct {
	Host               string   `json:"host" yaml:"host"`
	Reachable          bool     `json:"reachable" yaml:"reachable"`
	Supported          bool     `json:"supported" yaml:"supported"`
	PendingUpdates     []string `json:"pending_updates,omitempty" yaml:"pending_updates,omitempty"`
	SecurityUpdates    []string `json:"security_updates,omitempty" yaml:"security_updates,omitempty"`
	RebootRequired     bool     `json:"reboot_required" yaml:"reboot_required"`
	FailedUnits        []string `json:"failed_units,omitempty" yaml:"failed_units,omitempty"`
	RootUsage          string   `json:"root_usage,omitempty" yaml:"root_usage,omitempty"`
	HeldPackages       []string `json:"held_packages,omitempty" yaml:"held_packages,omitempty"`
	BrokenDependencies []string `json:"broken_dependencies,omitempty" yaml:"broken_dependencies,omitempty"`
	EvidenceComplete   bool     `json:"evidence_complete" yaml:"evidence_complete"`
	Warnings           []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// InterpretCheckUpdates converts an adapter run of the check-updates
// operation into per-host statuses. Hosts that failed or were unreachable
// are reported as such, never silently dropped.
func InterpretCheckUpdates(res *ansible.RunResult) []HostStatus {
	if res == nil {
		return nil
	}
	statuses := make([]HostStatus, 0, len(res.Hosts))
	for _, hr := range res.Hosts {
		hs := HostStatus{
			Host:             hr.Host,
			Reachable:        hr.Unreachable == 0,
			Supported:        true,
			EvidenceComplete: res.ParseError == "" && res.Success && res.EvidenceComplete && !hr.Failed && hr.Failures == 0 && hr.Unreachable == 0,
		}
		if res.ParseError != "" || !res.Success || !res.EvidenceComplete {
			hs.EvidenceComplete = false
			hs.Warnings = append(hs.Warnings, "preflight evidence or run status was incomplete")
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
		seen := map[string]bool{}
		for _, outcome := range res.TaskOutcomes[hr.Host] {
			seen[outcome.EvidenceID] = true
			interpretOutcome(&hs, outcome)
		}
		if len(res.TaskOutcomes[hr.Host]) == 0 {
			hs.EvidenceComplete = false
			hs.Warnings = append(hs.Warnings, "no task evidence was returned")
		} else {
			required := []string{taskDebianAssert, taskRefresh, taskUpgradable, taskUpgradeSim, taskRebootRequired, taskFailedUnits, taskRootUsage}
			if len(res.RequiredEvidence) > 0 {
				required = res.RequiredEvidence
			}
			for _, task := range required {
				if !seen[task] {
					hs.EvidenceComplete = false
					hs.Warnings = append(hs.Warnings, "missing task evidence: "+task)
				}
			}
		}
		statuses = append(statuses, hs)
	}
	return statuses
}

func interpretOutcome(hs *HostStatus, o ansible.TaskOutcome) {
	if o.Failed || o.Skipped || o.Unreachable || (o.RC != nil && *o.RC != 0) {
		hs.EvidenceComplete = false
		if o.Unreachable {
			hs.Warnings = append(hs.Warnings, "task reported unreachable")
		} else if o.Skipped {
			hs.Warnings = append(hs.Warnings, "task was skipped")
		} else {
			hs.Warnings = append(hs.Warnings, "task reported failure: "+o.Task)
		}
	}
	switch o.EvidenceID {
	case taskConnectivity:
		// Connectivity is represented by the host result; there is no payload
		// to interpret here.
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

// Snapshot converts a host status and its immutable inventory identity into
// the normalized facts used by plan comparison.
func Snapshot(host PlanHost, status HostStatus, sshUser string, sshPort int, keyConfigured, knownHostsConfigured bool) HostSnapshot {
	return HostSnapshot{
		Name: host.Name, Address: host.Address, Role: host.Role,
		Environment: host.Environment, PVENode: host.PVENode, PVEProfile: host.PVEProfile,
		PBSProfile: host.PBSProfile, Group: host.Group, AutomaticReboot: host.AutomaticReboot,
		Criticality: host.Criticality, SSHUser: sshUser, SSHPort: sshPort,
		KeyConfigured: keyConfigured, KnownHostsConfigured: knownHostsConfigured,
		Reachable: status.Reachable, Supported: status.Supported,
		PendingUpdates: append([]string(nil), status.PendingUpdates...), SecurityUpdates: append([]string(nil), status.SecurityUpdates...),
		HeldPackages: append([]string(nil), status.HeldPackages...), BrokenDependencies: append([]string(nil), status.BrokenDependencies...),
		RebootRequired: status.RebootRequired, FailedUnits: append([]string(nil), status.FailedUnits...), RootUsage: status.RootUsage,
		EvidenceComplete: status.EvidenceComplete,
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
