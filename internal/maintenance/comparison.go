package maintenance

import (
	"fmt"
	"sort"
	"strings"
)

// Disposition is the safe decision for one planned host.
type Disposition string

const (
	DispositionMatch   Disposition = "match"
	DispositionWarning Disposition = "warning"
	DispositionBlocked Disposition = "blocked"
	DispositionUnknown Disposition = "unknown"
	DispositionSkipped Disposition = "skipped"
)

// HostComparison contains field-level evidence for one host.
type HostComparison struct {
	Host          string      `json:"host" yaml:"host"`
	Disposition   Disposition `json:"disposition" yaml:"disposition"`
	Matching      []string    `json:"matching,omitempty" yaml:"matching,omitempty"`
	Warnings      []string    `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Blockers      []string    `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	ChangedFields []string    `json:"changed_fields,omitempty" yaml:"changed_fields,omitempty"`
	UnknownFields []string    `json:"unknown_fields,omitempty" yaml:"unknown_fields,omitempty"`
}

// Comparison is the complete typed plan/current-state comparison. It is safe
// to display and persist: values are field names and normalized dispositions,
// never credentials or raw provider output.
type Comparison struct {
	Schema        int              `json:"schema" yaml:"schema"`
	Matching      []string         `json:"matching,omitempty" yaml:"matching,omitempty"`
	Warnings      []string         `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Blockers      []string         `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	ChangedFields []string         `json:"changed_fields,omitempty" yaml:"changed_fields,omitempty"`
	UnknownFields []string         `json:"unknown_fields,omitempty" yaml:"unknown_fields,omitempty"`
	Hosts         []HostComparison `json:"hosts" yaml:"hosts"`
	Overall       Disposition      `json:"overall" yaml:"overall"`
}

const ComparisonSchemaVersion = 1

// CompareSnapshots compares all normalized safety fields. Missing safety
// evidence is unknown and therefore blocking; it is never treated as equal.
func CompareSnapshots(planned, current SafetySnapshot) Comparison {
	c := Comparison{Schema: ComparisonSchemaVersion, Overall: DispositionMatch}
	addUnique := func(dst *[]string, value string) {
		if value == "" {
			return
		}
		for _, existing := range *dst {
			if existing == value {
				return
			}
		}
		*dst = append(*dst, value)
	}
	if planned.Version == 0 || current.Version == 0 {
		c.UnknownFields = append(c.UnknownFields, "snapshot.version")
		c.Blockers = append(c.Blockers, "safety snapshot is missing")
	}
	if !planned.EvidenceComplete || !current.EvidenceComplete {
		c.UnknownFields = append(c.UnknownFields, "snapshot.evidence_complete")
		c.Blockers = append(c.Blockers, "required safety evidence is incomplete")
	}
	if planned.Infrastructure.Environment != current.Infrastructure.Environment {
		addUnique(&c.ChangedFields, "infrastructure.environment")
	}
	compareString(&c, "infrastructure.overall", planned.Infrastructure.Overall, current.Infrastructure.Overall)
	compareBool(&c, "infrastructure.maintenance_safe", planned.Infrastructure.MaintenanceSafe, current.Infrastructure.MaintenanceSafe)
	compareBool(&c, "backup.satisfied", planned.Backup.Satisfied, current.Backup.Satisfied)
	compareBool(&c, "backup.coverage_complete", planned.Backup.CoverageComplete, current.Backup.CoverageComplete)
	compareBool(&c, "backup.verification_healthy", planned.Backup.VerificationHealthy, current.Backup.VerificationHealthy)
	compareInt(&c, "backup.max_age_hours", planned.Backup.MaxAgeHours, current.Backup.MaxAgeHours)
	compareInt(&c, "backup.datastore_capacity_percent", planned.Backup.DatastoreCapacityPercent, current.Backup.DatastoreCapacityPercent)
	compareInt(&c, "backup.oldest_backup_age_hours", planned.Backup.OldestBackupAgeHours, current.Backup.OldestBackupAgeHours)
	if !equalStrings(planned.Backup.RequiredHosts, current.Backup.RequiredHosts) {
		c.ChangedFields = append(c.ChangedFields, "backup.required_hosts")
		c.Blockers = append(c.Blockers, "material state changed: backup.required_hosts")
	} else {
		c.Matching = append(c.Matching, "backup.required_hosts")
	}
	for name, status := range planned.Infrastructure.Checks {
		actual, ok := current.Infrastructure.Checks[name]
		if !ok {
			c.UnknownFields = append(c.UnknownFields, "infrastructure.checks."+name)
			continue
		}
		if status == actual {
			c.Matching = append(c.Matching, "infrastructure.checks."+name)
		} else {
			c.ChangedFields = append(c.ChangedFields, "infrastructure.checks."+name)
			c.Blockers = append(c.Blockers, "material state changed: infrastructure.checks."+name)
		}
	}
	if !current.Infrastructure.EvidenceComplete {
		c.UnknownFields = append(c.UnknownFields, "infrastructure")
	}
	for name := range planned.Hosts {
		ph, pok := planned.Hosts[name]
		ch, cok := current.Hosts[name]
		hc := HostComparison{Host: name, Disposition: DispositionMatch}
		if !cok {
			hc.Disposition = DispositionUnknown
			hc.UnknownFields = append(hc.UnknownFields, "host")
			hc.Blockers = append(hc.Blockers, "host safety state missing")
		}
		if pok && cok {
			compareHost(&hc, ph, ch)
		}
		sort.Strings(hc.Matching)
		sort.Strings(hc.Warnings)
		sort.Strings(hc.Blockers)
		sort.Strings(hc.ChangedFields)
		sort.Strings(hc.UnknownFields)
		c.Hosts = append(c.Hosts, hc)
		for _, v := range hc.Matching {
			addUnique(&c.Matching, name+"."+v)
		}
		for _, v := range hc.Warnings {
			addUnique(&c.Warnings, name+": "+v)
		}
		for _, v := range hc.Blockers {
			addUnique(&c.Blockers, name+": "+v)
		}
		for _, v := range hc.ChangedFields {
			addUnique(&c.ChangedFields, name+"."+v)
		}
		for _, v := range hc.UnknownFields {
			addUnique(&c.UnknownFields, name+"."+v)
		}
	}
	for name := range current.Hosts {
		if _, ok := planned.Hosts[name]; !ok {
			c.ChangedFields = append(c.ChangedFields, "hosts."+name)
		}
	}
	sort.Slice(c.Hosts, func(i, j int) bool { return c.Hosts[i].Host < c.Hosts[j].Host })
	sort.Strings(c.Matching)
	sort.Strings(c.Warnings)
	sort.Strings(c.Blockers)
	sort.Strings(c.ChangedFields)
	sort.Strings(c.UnknownFields)
	if len(c.UnknownFields) > 0 {
		c.Overall = DispositionUnknown
	} else if len(c.Blockers) > 0 || len(c.ChangedFields) > 0 {
		c.Overall = DispositionBlocked
	}
	return c
}

func compareHost(c *HostComparison, p, n HostSnapshot) {
	compare := func(field string, equal bool) {
		if equal {
			c.Matching = append(c.Matching, field)
		} else {
			c.ChangedFields = append(c.ChangedFields, field)
			c.Blockers = append(c.Blockers, "material state changed: "+field)
		}
	}
	compare("address", p.Address == n.Address)
	compare("role", p.Role == n.Role)
	compare("environment", p.Environment == n.Environment)
	compare("pve_node", p.PVENode == n.PVENode)
	compare("pve_profile", p.PVEProfile == n.PVEProfile)
	compare("pbs_profile", p.PBSProfile == n.PBSProfile)
	compare("group", p.Group == n.Group)
	compare("criticality", p.Criticality == n.Criticality)
	compare("automatic_reboot", p.AutomaticReboot == n.AutomaticReboot)
	compare("ssh_user", p.SSHUser == n.SSHUser)
	compare("ssh_port", p.SSHPort == n.SSHPort)
	compare("key_configured", p.KeyConfigured == n.KeyConfigured)
	compare("known_hosts_configured", p.KnownHostsConfigured == n.KnownHostsConfigured)
	compare("reachable", p.Reachable == n.Reachable)
	compare("supported", p.Supported == n.Supported)
	compare("pending_updates", equalStrings(p.PendingUpdates, n.PendingUpdates))
	compare("security_updates", equalStrings(p.SecurityUpdates, n.SecurityUpdates))
	compare("held_packages", equalStrings(p.HeldPackages, n.HeldPackages))
	compare("broken_dependencies", equalStrings(p.BrokenDependencies, n.BrokenDependencies))
	compare("reboot_required", p.RebootRequired == n.RebootRequired)
	compare("failed_units", equalStrings(p.FailedUnits, n.FailedUnits))
	compare("root_usage", p.RootUsage == n.RootUsage)
	if !p.EvidenceComplete || !n.EvidenceComplete {
		c.UnknownFields = append(c.UnknownFields, "evidence")
	}
	if n.RebootRequired {
		c.Warnings = append(c.Warnings, "reboot is required")
	}
	if len(n.FailedUnits) > 0 {
		c.Blockers = append(c.Blockers, "failed systemd units present")
	}
	if len(n.BrokenDependencies) > 0 {
		c.Blockers = append(c.Blockers, "broken dependencies present")
	}
	if n.RootUsage == "" {
		c.UnknownFields = append(c.UnknownFields, "root_usage")
	}
	if n.Reachable && !n.Supported {
		c.Blockers = append(c.Blockers, "unsupported operating system")
	}
	if len(c.UnknownFields) > 0 {
		c.Disposition = DispositionUnknown
	} else if len(c.Blockers) > 0 || len(c.ChangedFields) > 0 {
		c.Disposition = DispositionBlocked
	}
}

func compareString(c *Comparison, field, a, b string) {
	if a == "" || b == "" {
		c.UnknownFields = append(c.UnknownFields, field)
		return
	}
	if a == b {
		c.Matching = append(c.Matching, field)
	} else {
		c.ChangedFields = append(c.ChangedFields, field)
		c.Blockers = append(c.Blockers, fmt.Sprintf("material state changed: %s", field))
	}
}
func compareBool(c *Comparison, field string, a, b bool) {
	if a == b {
		c.Matching = append(c.Matching, field)
	} else {
		c.ChangedFields = append(c.ChangedFields, field)
		c.Blockers = append(c.Blockers, fmt.Sprintf("material state changed: %s", field))
	}
}
func compareInt(c *Comparison, field string, a, b int) {
	if a == b {
		c.Matching = append(c.Matching, field)
	} else {
		c.ChangedFields = append(c.ChangedFields, field)
		c.Blockers = append(c.Blockers, fmt.Sprintf("material state changed: %s", field))
	}
}
func equalStrings(a, b []string) bool {
	aa, bb := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	return strings.Join(aa, "\x00") == strings.Join(bb, "\x00")
}
