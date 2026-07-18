// Package ansible is Nodex's execution boundary for Linux maintenance. It
// invokes ansible-playbook as an external process — never through a shell —
// and only ever runs operations from the embedded allowlist below. Users
// select operation identifiers; there is no way to supply a playbook path,
// module name, inventory script, callback plugin, extra argument, or
// environment variable through Nodex.
package ansible

import (
	_ "embed"
	"fmt"
	"sort"

	"github.com/geoffmcc/nodex/internal/safety"
)

//go:embed playbooks/check-updates.yml
var checkUpdatesPlaybook string

//go:embed playbooks/verify-host.yml
var verifyHostPlaybook string

// Operation is one allowlisted maintenance operation backed by an embedded
// playbook. The playbook content ships inside the Nodex binary; paths on
// disk are never accepted.
type Operation struct {
	// ID is the operation identifier users select.
	ID string

	// Description is the human-readable summary.
	Description string

	// Safety is the confirmation tier the operation requires.
	Safety safety.Tier

	// ReadOnly operations make no changes to managed hosts (metadata cache
	// refreshes excepted where noted in the playbook).
	ReadOnly bool

	// RequiresBecome is true when any task escalates privileges.
	RequiresBecome bool

	// playbook is the embedded playbook content.
	playbook string
}

// Playbook returns the embedded playbook content.
func (o Operation) Playbook() string { return o.playbook }

// registry is the complete allowlist. The set grows only through code
// review: further operations (install-security-updates,
// install-approved-updates, restart-approved-services,
// reboot-approved-host, verify-service, configure-security-updates) are
// added in later phases together with the safety machinery that gates them.
var registry = map[string]Operation{
	"check-updates": {
		ID:             "check-updates",
		Description:    "Collect pending updates, reboot-required state, and failed units (read-only)",
		Safety:         safety.TierObservation,
		ReadOnly:       true,
		RequiresBecome: true, // apt metadata refresh only
		playbook:       checkUpdatesPlaybook,
	},
	"verify-host": {
		ID:             "verify-host",
		Description:    "Verify connectivity and basic host health (read-only)",
		Safety:         safety.TierObservation,
		ReadOnly:       true,
		RequiresBecome: false,
		playbook:       verifyHostPlaybook,
	},
}

// Lookup returns the allowlisted operation with the given ID.
func Lookup(id string) (Operation, error) {
	op, ok := registry[id]
	if !ok {
		return Operation{}, fmt.Errorf("unknown maintenance operation %q (allowed: %v)", id, OperationIDs())
	}
	return op, nil
}

// OperationIDs returns the sorted allowlist.
func OperationIDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Operations returns all allowlisted operations sorted by ID.
func Operations() []Operation {
	ops := make([]Operation, 0, len(registry))
	for _, id := range OperationIDs() {
		ops = append(ops, registry[id])
	}
	return ops
}
