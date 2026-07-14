package cli

import (
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/safety"
)

func TestOperations_Count(t *testing.T) {
	ops := Operations()
	if len(ops) < 80 {
		t.Errorf("expected at least 80 operations, got %d", len(ops))
	}
	// Quick sanity: should have more inspection than mutation ops.
	inspect := InspectionOperations()
	mutate := MutationOperations()
	t.Logf("Registry: %d total, %d inspection, %d mutation", len(ops), len(inspect), len(mutate))
}

func TestOperations_EveryMutationHasTierAboveObservation(t *testing.T) {
	for _, op := range MutationOperations() {
		if op.SafetyTier == safety.TierObservation {
			t.Errorf("mutation %q has TierObservation", op.Path)
		}
	}
}

func TestOperations_EveryInspectionHasTierObservation(t *testing.T) {
	for _, op := range InspectionOperations() {
		if op.SafetyTier != safety.TierObservation {
			t.Errorf("inspection %q has tier %s, want observation", op.Path, op.SafetyTier)
		}
	}
}

func TestOperations_NoInspectionProducesUPID(t *testing.T) {
	for _, op := range InspectionOperations() {
		if op.ProducesUPID {
			t.Errorf("inspection %q claims to produce UPID", op.Path)
		}
	}
}

func TestOperations_TypeConfirmRequiresDestructiveOrHigher(t *testing.T) {
	for _, op := range Operations() {
		if op.RequiresTypeConfirm && op.SafetyTier < safety.TierDestructive {
			t.Errorf("%q requires type confirm but tier is %s", op.Path, op.SafetyTier)
		}
	}
}

func TestOperations_ExpertRequiresSecurityAdmin(t *testing.T) {
	for _, op := range Operations() {
		if op.RequiresExpert && op.SafetyTier != safety.TierSecurityAdmin {
			t.Errorf("%q requires expert but tier is %s", op.Path, op.SafetyTier)
		}
	}
}

func TestOperations_LookupKnown(t *testing.T) {
	op := LookupOperation("vm start")
	if op == nil {
		t.Fatal("LookupOperation('vm start') returned nil")
	}
	if op.Inspection {
		t.Error("vm start should not be inspection")
	}
	if op.SafetyTier != safety.TierReversible {
		t.Errorf("vm start tier = %s, want reversible", op.SafetyTier)
	}
	if !op.ProducesUPID {
		t.Error("vm start should produce UPID")
	}
	if !op.UsesOperationResult {
		t.Error("vm start should use OperationResult")
	}
	if !op.Waitable {
		t.Error("vm start should be waitable")
	}
	if op.Scope != ScopeGuest {
		t.Errorf("vm start scope = %s, want guest", op.Scope)
	}
}

func TestOperations_LookupUnknown(t *testing.T) {
	op := LookupOperation("nonexistent command")
	if op != nil {
		t.Errorf("LookupOperation for unknown command should be nil, got %v", op)
	}
}

func TestOperations_DestructiveOpsHaveRightTier(t *testing.T) {
	destructive := []string{
		"vm delete",
		"container delete",
		"vm snapshot delete",
		"container snapshot delete",
		"storage delete",
		"backup job delete",
	}
	for _, path := range destructive {
		op := LookupOperation(path)
		if op == nil {
			t.Errorf("missing operation: %s", path)
			continue
		}
		if op.SafetyTier != safety.TierDestructive {
			t.Errorf("%s tier = %s, want destructive", path, op.SafetyTier)
		}
		if !op.RequiresTypeConfirm {
			t.Errorf("%s should require type confirm", path)
		}
	}
}

func TestOperations_DisruptiveOpsHaveRightTier(t *testing.T) {
	disruptive := []string{
		"vm reset",
		"vm reboot",
		"container reboot",
		"vm migrate",
		"container migrate",
		"vm clone",
		"container clone",
		"vm disk resize",
		"vm disk move",
		"vm template",
		"container template",
		"vm snapshot rollback",
		"container snapshot rollback",
		"storage upload",
		"backup create",
		"backup restore",
		"backup job create",
		"backup job update",
		"network apply",
		"network revert",
	}
	for _, path := range disruptive {
		op := LookupOperation(path)
		if op == nil {
			t.Errorf("missing operation: %s", path)
			continue
		}
		if op.SafetyTier != safety.TierDisruptive {
			t.Errorf("%s tier = %s, want disruptive", path, op.SafetyTier)
		}
	}
}

func TestOperations_SecurityAdminOps(t *testing.T) {
	adminOps := []string{
		"access user create",
		"access user delete",
		"access acl add",
	}
	for _, path := range adminOps {
		op := LookupOperation(path)
		if op == nil {
			t.Errorf("missing operation: %s", path)
			continue
		}
		if op.SafetyTier != safety.TierSecurityAdmin {
			t.Errorf("%s tier = %s, want security_admin", path, op.SafetyTier)
		}
		if !op.RequiresExpert {
			t.Errorf("%s should require expert", path)
		}
	}
}

func TestOperations_ReadOnlyOps(t *testing.T) {
	readOnly := []string{
		"version",
		"completion",
		"status",
		"node list",
		"node show",
		"vm list",
		"vm show",
		"vm config",
		"vm snapshots",
		"container list",
		"container show",
		"container config",
		"container snapshots",
		"storage list",
		"storage show",
		"storage content",
		"cluster status",
		"cluster log",
		"event list",
		"log",
		"doctor",
		"task list",
		"task show",
		"backup list",
		"backup content",
		"backup job list",
		"backup job show",
		"firewall list",
		"ha list",
		"ha groups",
		"ha status",
		"ha current",
		"sdn zones",
		"sdn vnets",
		"pools list",
		"profile list",
		"profile show",
		"profile current",
		"profile test",
		"profile export",
		"provider list",
		"provider capabilities",
		"ceph status",
		"ceph osd list",
		"ceph mon list",
		"ceph pool list",
		"replication list",
		"replication show",
		"access users list",
		"access groups list",
		"access roles list",
		"access acl list",
		"access domains list",
		"access tokens list",
	}
	for _, path := range readOnly {
		op := LookupOperation(path)
		if op == nil {
			t.Errorf("missing operation: %s", path)
			continue
		}
		if !op.Inspection {
			t.Errorf("%s should be inspection", path)
		}
		if op.SafetyTier != safety.TierObservation {
			t.Errorf("%s tier = %s, want observation", path, op.SafetyTier)
		}
	}
}

func TestOperations_WaitableOps(t *testing.T) {
	waitable := []string{
		"vm start", "vm stop", "vm shutdown", "vm reset", "vm reboot",
		"vm suspend", "vm resume", "vm pause", "vm unpause",
		"vm update", "vm cloud-init", "vm delete",
		"vm migrate", "vm clone", "vm disk resize", "vm disk move", "vm template",
		"container start", "container stop", "container shutdown", "container reboot",
		"container suspend", "container resume",
		"container update", "container delete",
		"container migrate", "container clone", "container template",
		"vm snapshot create", "vm snapshot delete", "vm snapshot rollback",
		"container snapshot create", "container snapshot delete", "container snapshot rollback",
		"storage upload", "storage delete",
		"backup create", "backup restore",
		"ceph osd create", "ceph osd destroy", "ceph pool create", "ceph pool destroy",
	}
	for _, path := range waitable {
		op := LookupOperation(path)
		if op == nil {
			t.Errorf("missing operation: %s", path)
			continue
		}
		if !op.Waitable {
			t.Errorf("%s should be waitable", path)
		}
	}
}

func TestValidateRegistry_NoErrors(t *testing.T) {
	errs := ValidateRegistry()
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("registry validation error: %v", err)
		}
	}
}

func TestOperationPathsAreUnique(t *testing.T) {
	seen := make(map[string]bool)
	for _, op := range Operations() {
		if seen[op.Path] {
			t.Errorf("duplicate operation path: %s", op.Path)
		}
		seen[op.Path] = true
	}
}

func TestOperations_TierStringMatchesSafetyPackage(t *testing.T) {
	for _, op := range Operations() {
		s := op.SafetyTier.String()
		valid := map[string]bool{
			"observation": true, "reversible": true, "disruptive": true,
			"destructive": true, "security_admin": true,
		}
		if !valid[s] {
			t.Errorf("%q has invalid tier string: %s", op.Path, s)
		}
	}
}

func TestOperations_DescriptionsNotEmpty(t *testing.T) {
	for _, op := range Operations() {
		if strings.TrimSpace(op.Description) == "" {
			t.Errorf("%q has empty description", op.Path)
		}
	}
}

func TestOperations_HandlerFuncNotEmpty(t *testing.T) {
	for _, op := range Operations() {
		if strings.TrimSpace(op.HandlerFunc) == "" {
			t.Errorf("%q has empty handler func", op.Path)
		}
	}
}

func TestOperations_OutputModesNotEmpty(t *testing.T) {
	for _, op := range Operations() {
		if len(op.OutputModes) == 0 {
			t.Errorf("%q has no output modes", op.Path)
		}
	}
}
