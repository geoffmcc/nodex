package cli

import (
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
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

func TestOperations_TypeConfirmPublishesTargetFormat(t *testing.T) {
	// P2-B: discovery must let a caller derive the confirmation value. A
	// type-in operation with no published format leaves the caller guessing.
	// The two guest operations below are the ones proven non-interactive in
	// non-interactive in the Nits-4 evidence.
	for _, op := range Operations() {
		if op.RequiresTypeConfirm && op.ConfirmTargetFormat == "" {
			t.Errorf("%q requires type confirm but publishes no ConfirmTargetFormat", op.Path)
		}
	}
}

func TestOperations_ConfirmTargetFormatMatchesHandlerTarget(t *testing.T) {
	// Pin the derivation rules the handlers actually use. A wrong format is
	// worse than none: it makes a correct caller refuse a valid operation.
	tests := []struct {
		path string
		want string
	}{
		{"vm delete", "node>/<vmid>"},
		{"container delete", "node>/<vmid>"},
		{"vm snapshot delete", "snapshot name only"},
		{"container snapshot delete", "snapshot name only"},
		{"storage delete", "volume ID only"},
		{"backup job delete", "backup job ID only"},
	}
	for _, tt := range tests {
		op := LookupOperation(tt.path)
		if op == nil {
			t.Fatalf("LookupOperation(%q) = nil", tt.path)
		}
		if !strings.Contains(op.ConfirmTargetFormat, tt.want) {
			t.Errorf("%q format = %q, want it to contain %q", tt.path, op.ConfirmTargetFormat, tt.want)
		}
	}
}

func TestOperations_ConfirmTargetFormatAbsentWithoutTypeConfirm(t *testing.T) {
	// A format on a non-type-in operation would imply a confirmation value
	// that is never compared.
	for _, op := range Operations() {
		if !op.RequiresTypeConfirm && op.ConfirmTargetFormat != "" {
			t.Errorf("%q does not require type confirm but publishes format %q", op.Path, op.ConfirmTargetFormat)
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

func TestUnsupportedVMLifecycleDiscoveryIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"vm pause", "vm suspend"},
		{"vm unpause", "vm resume"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			op := LookupOperation(tc.path)
			if op == nil {
				t.Fatalf("LookupOperation(%q) = nil", tc.path)
			}
			if !strings.HasPrefix(op.ProviderSupportNote, "Unsupported:") || !strings.Contains(op.ProviderSupportNote, tc.want) {
				t.Fatalf("provider support note = %q, want unsupported guidance containing %q", op.ProviderSupportNote, tc.want)
			}
			contract := describeOperation(*op, true)
			if contract.AgentSupported || contract.AgentUnsupportedWhy == "" {
				t.Fatalf("unsupported VM operation was advertised for agent execution: %+v", contract)
			}
			if contract.SubmitsTask || contract.ProviderInterface != "" || op.UsesOperationResult {
				t.Fatalf("unsupported VM operation claims provider execution: %+v", contract)
			}
			if len(contract.RemoteSideEffects) != 0 || !strings.Contains(contract.ProviderPermissionNote, "Not checked") {
				t.Fatalf("unsupported VM operation claims a remote request or permission check: %+v", contract)
			}
			if op.Waitable || op.ProducesUPID {
				t.Fatalf("unsupported VM operation claims task polling: %+v", op)
			}
			summary := summarizeOperation(*op)
			if !strings.Contains(summary.ProviderSupport, tc.want) {
				t.Fatalf("operation list provider support = %q, want guidance containing %q", summary.ProviderSupport, tc.want)
			}
			if !strings.Contains(summary.ProviderPermission, "not checked") {
				t.Fatalf("operation list permission status = %q, want unsupported/no-request disclosure", summary.ProviderPermission)
			}
		})
	}
}

func TestGuestCreateResourceFlagsAreAdvertised(t *testing.T) {
	tests := []struct {
		path        string
		flags       []string
		defaults    map[string]any
		constraints map[string][2]any
	}{
		{
			path:  "vm create",
			flags: []string{"--cores", "--memory", "--disk-size", "--disk-storage"},
			defaults: map[string]any{
				"--cores":     domain.DefaultVMCreateCores,
				"--memory":    domain.DefaultVMCreateMemoryMiB,
				"--disk-size": domain.DefaultVMCreateDiskSizeGiB,
			},
			constraints: map[string][2]any{
				"--cores":     {1, domain.MaxCreateCores},
				"--memory":    {16, domain.MaxCreateMemoryMiB},
				"--disk-size": {1, domain.MaxCreateDiskSizeGiB},
			},
		},
		{
			path:  "container create",
			flags: []string{"--cores", "--memory", "--swap", "--rootfs-size", "--rootfs-storage"},
			defaults: map[string]any{
				"--memory": domain.DefaultContainerCreateMemoryMiB,
				"--swap":   domain.DefaultContainerCreateSwapMiB,
			},
			constraints: map[string][2]any{
				"--cores":       {1, domain.MaxCreateCores},
				"--memory":      {16, domain.MaxCreateMemoryMiB},
				"--swap":        {0, domain.MaxCreateMemoryMiB},
				"--rootfs-size": {1, domain.MaxCreateDiskSizeGiB},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			op := LookupOperation(tt.path)
			if op == nil {
				t.Fatalf("LookupOperation(%q) = nil", tt.path)
			}
			contract := describeOperation(*op, true)
			got := make(map[string]AgentFlag, len(contract.Flags))
			for _, flag := range contract.Flags {
				got[flag.Name] = flag
			}
			for _, flag := range tt.flags {
				meta, ok := got[flag]
				if !ok {
					t.Errorf("%q missing from described flags: %+v", flag, contract.Flags)
					continue
				}
				if want, exists := tt.defaults[flag]; exists && meta.Default != want {
					t.Errorf("%s default = %#v, want %#v", flag, meta.Default, want)
				}
				if want, exists := tt.constraints[flag]; exists && (meta.Minimum != want[0] || meta.Maximum != want[1]) {
					t.Errorf("%s bounds = %v..%v, want %v..%v", flag, meta.Minimum, meta.Maximum, want[0], want[1])
				}
			}
			if len(contract.Verification) == 0 || !strings.Contains(strings.Join(contract.Verification, " "), "read back") {
				t.Errorf("%s verification discovery does not describe provider readback: %v", tt.path, contract.Verification)
			}
		})
	}
}

func TestOperations_LookupUnknown(t *testing.T) {
	op := LookupOperation("nonexistent command")
	if op != nil {
		t.Errorf("LookupOperation for unknown command should be nil, got %v", op)
	}
}

func TestOperations_ReturnCopies(t *testing.T) {
	ops := Operations()
	if len(ops) == 0 {
		t.Fatal("operation registry is empty")
	}
	original := LookupOperation(ops[0].Path)
	if original == nil {
		t.Fatalf("operation %q disappeared from registry", ops[0].Path)
	}

	ops[0].Path = "tampered"
	if len(ops[0].OutputModes) > 0 {
		ops[0].OutputModes[0] = "tampered"
	}
	if len(ops[0].RiskDimensions) > 0 {
		ops[0].RiskDimensions[0] = RiskDataLoss
	}
	lookup := LookupOperation(original.Path)
	if lookup == nil || lookup.Path != original.Path {
		t.Fatalf("registry was changed through Operations: %+v", lookup)
	}
	if len(original.OutputModes) > 0 && lookup.OutputModes[0] != original.OutputModes[0] {
		t.Fatalf("output modes were changed through Operations: %v", lookup.OutputModes)
	}
	if len(original.RiskDimensions) > 0 && lookup.RiskDimensions[0] != original.RiskDimensions[0] {
		t.Fatalf("risk dimensions were changed through Operations: %v", lookup.RiskDimensions)
	}

	lookup.Path = "tampered"
	if len(lookup.OutputModes) > 0 {
		lookup.OutputModes[0] = "tampered"
	}
	if current := LookupOperation(original.Path); current == nil || current.Path != original.Path || (len(original.OutputModes) > 0 && current.OutputModes[0] != original.OutputModes[0]) {
		t.Fatalf("registry was changed through LookupOperation: %+v", current)
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
		"vm clone", "vm create", "container create", "container restore",
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
		"firewall cluster-rules",
		"ha list",
		"ha groups",
		"ha status",
		"ha current",
		"sdn zones",
		"sdn vnets",
		"sdn subnets",
		"sdn controllers",
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
		"vm suspend", "vm resume",
		"vm update", "vm cloud-init", "vm delete",
		"vm migrate", "vm clone", "vm create", "vm disk resize", "vm disk move", "vm template",
		"container start", "container stop", "container shutdown", "container reboot",
		"container suspend", "container resume",
		"container update", "container delete",
		"container migrate", "container clone", "container create", "container restore", "container template",
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

func TestCommandTree_EveryReachableHandlerHasBoundMetadata(t *testing.T) {
	var walk func(map[string]*command, string)
	walk = func(nodes map[string]*command, prefix string) {
		for _, cmd := range nodes {
			path := cmd.name
			if prefix != "" {
				path = prefix + " " + cmd.name
			}
			if cmd.run != nil {
				if !cmd.bound || cmd.meta == nil {
					t.Errorf("reachable command %q is not bound to operation metadata", path)
				}
			}
			if cmd.sub != nil {
				walk(cmd.sub, path)
			}
		}
	}
	walk(commands, "")
}

func TestOperations_AliasesAndPathsAreUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, op := range Operations() {
		for _, path := range append([]string{op.Path}, op.Aliases...) {
			if previous, ok := seen[path]; ok {
				t.Errorf("operation path %q is claimed by %q and %q", path, previous, op.Path)
			}
			seen[path] = op.Path
		}
	}
}

func TestOperations_TypeConfirmContractPublishesTargetFormat(t *testing.T) {
	// P2-B acceptance: help, discovery metadata, and execution must agree on
	// the confirmation value. This pins the two machine-readable surfaces an
	// agent actually consumes; the help side is pinned in help_test.go.
	for _, op := range Operations() {
		if !op.RequiresTypeConfirm {
			continue
		}
		c := describeOperation(op, false)
		if c.ConfirmationTargetFormat != op.ConfirmTargetFormat {
			t.Errorf("%q: contract format %q != registry format %q",
				op.Path, c.ConfirmationTargetFormat, op.ConfirmTargetFormat)
		}
		if op.ConfirmTargetFormat == "" {
			t.Errorf("%q: contract exposes an empty confirmation target format", op.Path)
		}
	}
}

func TestOperations_TypeConfirmContractStatesRuntimeResolution(t *testing.T) {
	// The format is derivable from arguments, but the concrete value is not.
	// A caller must not treat the published format as a predicted target.
	for _, op := range Operations() {
		if !op.RequiresTypeConfirm {
			continue
		}
		c := describeOperation(op, false)
		joined := strings.Join(c.Constraints, "\n")
		if !strings.Contains(joined, "not predicted by this contract") {
			t.Errorf("%q: constraints omit the runtime-resolution caveat", op.Path)
		}
		if !strings.Contains(joined, op.ConfirmTargetFormat) {
			t.Errorf("%q: constraints omit the published format", op.Path)
		}
	}
}
