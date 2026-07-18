package maintenance

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// PlanSchemaVersion versions the serialized plan shape.
const PlanSchemaVersion = 1

// DefaultPlanTTL is how long a plan stays applicable.
const DefaultPlanTTL = 4 * time.Hour

// Update policies. A full upgrade still operates only within the currently
// configured distribution release and repositories.
const (
	PolicySecurityOnly = "security-only"
	PolicyApprovedFull = "approved-full-upgrade"
)

// ValidPolicy reports whether p is a known update policy.
func ValidPolicy(p string) bool {
	return p == PolicySecurityOnly || p == PolicyApprovedFull
}

// RebootPolicyNever is the only reboot policy Phase 5 emits: plans never
// authorize reboots. Explicit reboot operations arrive in a later phase.
const RebootPolicyNever = "never"

// PlanHost is one target host inside a plan.
type PlanHost struct {
	Name            string   `json:"name" yaml:"name"`
	Address         string   `json:"address" yaml:"address"`
	Role            string   `json:"role" yaml:"role"`
	Group           string   `json:"group,omitempty" yaml:"group,omitempty"`
	Criticality     string   `json:"criticality" yaml:"criticality"`
	BackupRequired  bool     `json:"backup_required" yaml:"backup_required"`
	PendingUpdates  []string `json:"pending_updates,omitempty" yaml:"pending_updates,omitempty"`
	SecurityUpdates []string `json:"security_updates,omitempty" yaml:"security_updates,omitempty"`
	RebootRequired  bool     `json:"reboot_required" yaml:"reboot_required"`
	Warnings        []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// BackupState captures the backup requirements and their observed
// satisfaction at planning time.
type BackupState struct {
	RequiredHosts []string `json:"required_hosts,omitempty" yaml:"required_hosts,omitempty"`
	MaxAgeHours   int      `json:"max_age_hours" yaml:"max_age_hours"`
	Satisfied     bool     `json:"satisfied" yaml:"satisfied"`
	Detail        string   `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// InfraSnapshot records the infrastructure state the plan was built
// against; apply (later phase) rejects plans whose environment has
// materially changed.
type InfraSnapshot struct {
	Environment     string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	Overall         string   `json:"overall,omitempty" yaml:"overall,omitempty"`
	MaintenanceSafe bool     `json:"maintenance_safe" yaml:"maintenance_safe"`
	Blockers        []string `json:"blockers,omitempty" yaml:"blockers,omitempty"`
}

// Plan is an immutable, expiring, tamper-evident maintenance plan. It
// contains no secrets and is safe to display and store. The Digest field
// covers every other field; any post-creation modification is detectable.
type Plan struct {
	Schema      int    `json:"schema" yaml:"schema"`
	PlanID      string `json:"plan_id" yaml:"plan_id"`
	CreatedAt   int64  `json:"created_at" yaml:"created_at"`
	ExpiresAt   int64  `json:"expires_at" yaml:"expires_at"`
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`

	// Policy is the planned update policy (security-only or
	// approved-full-upgrade).
	Policy string `json:"policy" yaml:"policy"`

	Hosts []PlanHost `json:"hosts" yaml:"hosts"`

	// HostOrder is the planned execution order: standard hosts first, then
	// critical hosts strictly serial; PVE, PBS, and DNS roles last.
	HostOrder []string `json:"host_order" yaml:"host_order"`

	// BatchSize applies to standard hosts; critical hosts are always
	// serial.
	BatchSize int `json:"batch_size" yaml:"batch_size"`

	RebootPolicy string `json:"reboot_policy" yaml:"reboot_policy"`

	// SafetyClassification is the tier apply must enforce.
	SafetyClassification string `json:"safety_classification" yaml:"safety_classification"`

	Backup   BackupState   `json:"backup" yaml:"backup"`
	Infra    InfraSnapshot `json:"infra" yaml:"infra"`
	Warnings []string      `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Blockers []string      `json:"blockers,omitempty" yaml:"blockers,omitempty"`

	// Digest is the hex SHA-256 over the canonical JSON of the plan with
	// this field empty.
	Digest string `json:"digest" yaml:"digest"`
}

// NewPlanID returns a random plan identifier.
func NewPlanID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate plan id: %w", err)
	}
	return "mp-" + hex.EncodeToString(b), nil
}

// Finalize normalizes ordering for determinism, computes the digest, and
// returns the completed plan.
func Finalize(p Plan) (Plan, error) {
	sort.Slice(p.Hosts, func(i, j int) bool { return p.Hosts[i].Name < p.Hosts[j].Name })
	sort.Strings(p.Warnings)
	sort.Strings(p.Blockers)
	sort.Strings(p.Backup.RequiredHosts)
	sort.Strings(p.Infra.Blockers)
	digest, err := computeDigest(p)
	if err != nil {
		return Plan{}, err
	}
	p.Digest = digest
	return p, nil
}

// computeDigest hashes the canonical JSON of the plan with Digest emptied.
func computeDigest(p Plan) (string, error) {
	p.Digest = ""
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("canonicalize plan: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Verify checks a plan's structural integrity, digest, and expiry against
// the given time. It returns an error describing the first failed check.
func Verify(p Plan, now time.Time) error {
	if p.Schema != PlanSchemaVersion {
		return fmt.Errorf("unsupported plan schema %d (expected %d)", p.Schema, PlanSchemaVersion)
	}
	if p.PlanID == "" {
		return fmt.Errorf("plan has no ID")
	}
	if p.Digest == "" {
		return fmt.Errorf("plan has no digest")
	}
	want, err := computeDigest(p)
	if err != nil {
		return err
	}
	if want != p.Digest {
		return fmt.Errorf("plan digest mismatch: the plan was modified after creation")
	}
	if p.ExpiresAt <= now.Unix() {
		return fmt.Errorf("plan expired at %s", time.Unix(p.ExpiresAt, 0).UTC().Format(time.RFC3339))
	}
	if !ValidPolicy(p.Policy) {
		return fmt.Errorf("unknown update policy %q", p.Policy)
	}
	if p.RebootPolicy != RebootPolicyNever {
		return fmt.Errorf("unsupported reboot policy %q", p.RebootPolicy)
	}
	return nil
}

// OrderHosts computes the planned execution order: standard-criticality
// hosts first (alphabetical), then critical non-infrastructure hosts, then
// PVE/PBS/DNS roles last — all critical hosts strictly serial in apply.
func OrderHosts(hosts []PlanHost) []string {
	rank := func(h PlanHost) int {
		infra := h.Role == "pve" || h.Role == "pbs" || h.Role == "dns"
		switch {
		case h.Criticality != "critical" && !infra:
			return 0
		case !infra:
			return 1
		default:
			return 2
		}
	}
	sorted := append([]PlanHost(nil), hosts...)
	sort.Slice(sorted, func(i, j int) bool {
		ri, rj := rank(sorted[i]), rank(sorted[j])
		if ri != rj {
			return ri < rj
		}
		return sorted[i].Name < sorted[j].Name
	})
	order := make([]string, 0, len(sorted))
	for _, h := range sorted {
		order = append(order, h.Name)
	}
	return order
}
