// Package cli provides the Nodex command-line interface, including the
// canonical operation registry that defines every reachable command's
// metadata: classification, safety tier, scope, and provider requirements.
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/safety"
)

// Scope classifies the operational domain of a command.
type Scope string

const (
	ScopeCluster  Scope = "cluster"
	ScopeNode     Scope = "node"
	ScopeGuest    Scope = "guest"
	ScopeStorage  Scope = "storage"
	ScopeProfile  Scope = "profile"
	ScopeSystem   Scope = "system"
	ScopeFirewall Scope = "firewall"
	ScopeNetwork  Scope = "network"
	ScopeAccess   Scope = "access"
	ScopeSDN      Scope = "sdn"
	ScopeCeph     Scope = "ceph"
	ScopeBackup   Scope = "backup"
	ScopeHA       Scope = "ha"
	ScopeRepl     Scope = "replication"
)

// RiskDimension describes a specific risk axis independent of safety tier.
type RiskDimension string

const (
	RiskNone         RiskDimension = ""
	RiskDataLoss     RiskDimension = "data_loss"
	RiskServiceDown  RiskDimension = "service_down"
	RiskNetworkLock  RiskDimension = "network_lockout"
	RiskDataExposure RiskDimension = "data_exposure"
	RiskPrivEsc      RiskDimension = "privilege_escalation"
)

// SecuritySensitivity classifies the security profile of a command separately
// from operational risk. This is not an ordinal tier but a label for the kind
// of security-relevant change the command makes.
type SecuritySensitivity string

const (
	SecNone        SecuritySensitivity = ""
	SecIdentity    SecuritySensitivity = "identity"
	SecAccess      SecuritySensitivity = "access_control"
	SecCredentials SecuritySensitivity = "credentials"
	SecNetwork     SecuritySensitivity = "network_policy"
	SecStorage     SecuritySensitivity = "storage_data"
)

// OperationMeta is the canonical metadata for a single Nodex command.
// Every reachable command path has an entry in the operation registry.
type OperationMeta struct {
	// Path is the full command path (e.g., "vm start", "snapshot delete").
	Path string

	// Aliases are alternative command paths that reach the same handler.
	Aliases []string

	// Description is a short human-readable summary.
	Description string

	// Inspection is true when the command is read-only (no state change).
	Inspection bool

	// Scope classifies the operational domain.
	Scope Scope

	// SafetyTier is the safety classification from the safety package.
	SafetyTier safety.Tier

	// RiskDimensions describe independent risk axes.
	RiskDimensions []RiskDimension

	// SecuritySensitivity describes the security profile (separate from risk).
	SecuritySensitivity SecuritySensitivity

	// RequiresTypeConfirm is true when typed-target verification is required.
	RequiresTypeConfirm bool

	// RequiresExpert is true when --expert is required (Tier 4).
	RequiresExpert bool

	// Waitable is true when --wait is supported for task polling.
	Waitable bool

	// ProducesUPID is true when the command returns a provider task UPID.
	ProducesUPID bool

	// UsesOperationResult is true when the command writes an OperationResult.
	UsesOperationResult bool

	// OutputModes lists the supported output formats.
	OutputModes []string

	// CapabilityInterface is the Go interface required for this operation.
	CapabilityInterface string

	// HandlerFunc is the registered handler function name (for documentation).
	HandlerFunc string
}

// operationRegistry is the canonical source of truth for all Nodex commands.
// It is built from the command registration tree and enriched with metadata.
var operationRegistry = buildRegistry()

// buildRegistry constructs the full operation registry from the command tree.
func buildRegistry() []OperationMeta {
	// Preallocate a reasonable capacity. The exact count is ~180 entries.
	ops := make([]OperationMeta, 0, 200)

	// --- version ---
	ops = append(ops, OperationMeta{
		Path: "version", Description: "Show version information",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runVersion",
	})
	ops = append(ops, OperationMeta{
		Path: "version compare", Description: "Compare two semver versions",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runVersionCompare",
	})
	ops = append(ops, OperationMeta{
		Path: "version parse", Description: "Parse a semver version",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runVersionParse",
	})

	// --- init, completion ---
	ops = append(ops, OperationMeta{
		Path: "init", Description: "Initialize nodex configuration",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		OutputModes: []string{"table"}, HandlerFunc: "runInit",
	})
	ops = append(ops, OperationMeta{
		Path: "completion", Description: "Generate shell completion scripts",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runCompletion",
	})

	// --- profile ---
	ops = append(ops, OperationMeta{
		Path: "profile add", Description: "Add a new profile",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileAdd",
	})
	ops = append(ops, OperationMeta{
		Path: "profile list", Description: "List all profiles",
		Inspection: true, Scope: ScopeProfile, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runProfileList",
	})
	ops = append(ops, OperationMeta{
		Path: "profile show", Description: "Show profile details",
		Inspection: true, Scope: ScopeProfile, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runProfileShow",
	})
	ops = append(ops, OperationMeta{
		Path: "profile set-credentials", Description: "Set profile credentials",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		SecuritySensitivity: SecCredentials,
		OutputModes:         []string{"table"}, HandlerFunc: "runProfileSetCredentials",
	})
	ops = append(ops, OperationMeta{
		Path: "profile use", Description: "Set the current profile",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileUse",
	})
	ops = append(ops, OperationMeta{
		Path: "profile current", Description: "Show the current profile",
		Inspection: true, Scope: ScopeProfile, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileCurrent",
	})
	ops = append(ops, OperationMeta{
		Path: "profile test", Description: "Test profile connectivity",
		Inspection: true, Scope: ScopeProfile, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileTest",
	})
	ops = append(ops, OperationMeta{
		Path: "profile remove", Description: "Remove a profile",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileRemove",
	})
	ops = append(ops, OperationMeta{
		Path: "profile export", Description: "Export a profile (sanitized)",
		Inspection: true, Scope: ScopeProfile, SafetyTier: safety.TierObservation,
		OutputModes: []string{"json"}, HandlerFunc: "runProfileExport",
	})
	ops = append(ops, OperationMeta{
		Path: "profile import", Description: "Import a profile from stdin",
		Inspection: false, Scope: ScopeProfile, SafetyTier: safety.TierReversible,
		OutputModes: []string{"table"}, HandlerFunc: "runProfileImport",
	})

	// --- provider ---
	ops = append(ops, OperationMeta{
		Path: "provider list", Description: "List available providers",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runProviderList",
	})
	ops = append(ops, OperationMeta{
		Path: "provider capabilities", Description: "Show provider capabilities",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runProviderCapabilities",
	})

	// --- status ---
	ops = append(ops, OperationMeta{
		Path: "status", Description: "Show cluster status overview",
		Inspection: true, Scope: ScopeCluster, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runStatus",
		CapabilityInterface: "ClusterInspector,VMInspector,ContainerInspector,StorageInspector",
	})

	// --- node ---
	nodeOps := []OperationMeta{
		{Path: "node list", Description: "List all nodes", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeInspector", HandlerFunc: "runNodeList"},
		{Path: "node show", Description: "Show node details", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeInspector", HandlerFunc: "runNodeShow"},
		{Path: "node status", Description: "Show detailed node status", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeStatus"},
		{Path: "node services", Description: "List node services", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeServices"},
		{Path: "node network", Description: "Show node network interfaces", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeNetwork"},
		{Path: "node dns", Description: "Show node DNS configuration", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeDNS"},
		{Path: "node time", Description: "Show node time configuration", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeTime"},
		{Path: "node disks", Description: "List node disks", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeDisks"},
		{Path: "node certificates", Description: "List node certificates", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeCertificates"},
		{Path: "node subscription", Description: "Show node subscription", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeSubscription"},
		{Path: "node updates", Description: "List available updates", Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNodeUpdates"},
	}
	ops = append(ops, nodeOps...)

	// --- vm (inspection) ---
	ops = append(ops, OperationMeta{
		Path: "vm list", Description: "List all virtual machines",
		Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "VMInspector", HandlerFunc: "runVMList",
	})
	ops = append(ops, OperationMeta{
		Path: "vm show", Description: "Show VM details",
		Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "VMInspector", HandlerFunc: "runVMShow",
	})
	ops = append(ops, OperationMeta{
		Path: "vm config", Description: "Show VM configuration",
		Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "VMInspector", HandlerFunc: "runVMConfig",
	})
	ops = append(ops, OperationMeta{
		Path: "vm snapshots", Description: "List VM snapshots",
		Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SnapshotInspector", HandlerFunc: "runVMSnapshots",
	})
	ops = append(ops, OperationMeta{
		Path: "vm snapshot-config", Description: "Show VM snapshot config",
		Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SnapshotDetailProvider", HandlerFunc: "runVMSnapshotConfig",
	})

	// --- vm (lifecycle mutations - Tier 1: reversible) ---
	vmLifecycleReversible := []struct{ op, desc string }{
		{"vm start", "Start a VM"},
		{"vm stop", "Stop a VM (force)"},
		{"vm shutdown", "Graceful VM shutdown"},
		{"vm suspend", "Suspend a VM to disk"},
		{"vm resume", "Resume a suspended VM"},
		{"vm pause", "Pause (freeze) a VM"},
		{"vm unpause", "Unpause a frozen VM"},
	}
	for _, v := range vmLifecycleReversible {
		ops = append(ops, OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
			RiskDimensions: []RiskDimension{RiskServiceDown},
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes:         []string{"table", "json", "yaml"},
			CapabilityInterface: "LifecycleProvider", HandlerFunc: "run" + toHandler(v.op),
		})
	}

	// --- vm (lifecycle mutations - Tier 2: disruptive) ---
	vmLifecycleDisruptive := []struct{ op, desc string }{
		{"vm reset", "Hard reset a VM"},
		{"vm reboot", "Reboot a VM"},
	}
	for _, v := range vmLifecycleDisruptive {
		ops = append(ops, OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
			RiskDimensions: []RiskDimension{RiskServiceDown},
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes:         []string{"table", "json", "yaml"},
			CapabilityInterface: "LifecycleProvider", HandlerFunc: "run" + toHandler(v.op),
		})
	}

	// --- vm (config mutations) ---
	ops = append(ops, OperationMeta{
		Path: "vm update", Description: "Update VM configuration",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
		RiskDimensions: []RiskDimension{RiskServiceDown},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "ConfigProvider", HandlerFunc: "runVMUpdate",
	})
	ops = append(ops, OperationMeta{
		Path: "vm cloud-init", Description: "Regenerate cloud-init config",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
		Waitable: true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "CloudInitProvider", HandlerFunc: "runVMCloudInit",
	})

	// --- vm (destructive mutations) ---
	ops = append(ops, OperationMeta{
		Path: "vm delete", Description: "Delete a VM (destructive)",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDestructive,
		RiskDimensions:      []RiskDimension{RiskDataLoss},
		RequiresTypeConfirm: true,
		Waitable:            true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "DeleteProvider", HandlerFunc: "runVMDelete",
	})

	// --- vm (disruptive mutations) ---
	vmDisruptive := []struct {
		op, desc, capIface, handler string
		risks                       []RiskDimension
	}{
		{"vm template", "Convert VM to template", "TemplateProvider", "runVMTemplate", []RiskDimension{RiskDataLoss}},
		{"vm migrate", "Migrate VM to another node", "MigrationProvider", "runVMMigrate", []RiskDimension{RiskServiceDown}},
		{"vm clone", "Clone a VM", "CloneProvider", "runVMClone", nil},
		{"vm disk resize", "Resize VM disk", "DiskProvider", "runVMDiskResize", []RiskDimension{RiskDataLoss}},
		{"vm disk move", "Move VM disk to another storage", "DiskProvider", "runVMDiskMove", []RiskDimension{RiskServiceDown}},
	}
	for _, v := range vmDisruptive {
		ops = append(ops, OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
			RiskDimensions: v.risks,
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes:         []string{"table", "json", "yaml"},
			CapabilityInterface: v.capIface, HandlerFunc: v.handler,
		})
	}

	// --- vm snapshot mutations ---
	ops = append(ops, OperationMeta{
		Path: "vm snapshot create", Description: "Create a VM snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
		Waitable: true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runVMSnapshotCreate",
	})
	ops = append(ops, OperationMeta{
		Path: "vm snapshot delete", Description: "Delete a VM snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDestructive,
		RiskDimensions:      []RiskDimension{RiskDataLoss},
		RequiresTypeConfirm: true,
		Waitable:            true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runVMSnapshotDelete",
	})
	ops = append(ops, OperationMeta{
		Path: "vm snapshot rollback", Description: "Rollback to a VM snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
		RiskDimensions: []RiskDimension{RiskDataLoss, RiskServiceDown},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runVMSnapshotRollback",
	})

	// --- container (inspection) ---
	ctInspection := []OperationMeta{
		{Path: "container list", Description: "List all containers", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ContainerInspector", HandlerFunc: "runContainerList"},
		{Path: "container show", Description: "Show container details", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ContainerInspector", HandlerFunc: "runContainerShow"},
		{Path: "container config", Description: "Show container configuration", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ContainerInspector", HandlerFunc: "runContainerConfig"},
		{Path: "container snapshots", Description: "List container snapshots", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SnapshotInspector", HandlerFunc: "runContainerSnapshots"},
		{Path: "container snapshot-config", Description: "Show container snapshot config", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SnapshotDetailProvider", HandlerFunc: "runContainerSnapshotConfig"},
	}
	ops = append(ops, ctInspection...)

	// --- container (lifecycle mutations - Tier 1: reversible) ---
	ctLifecycleReversible := []struct{ op, desc string }{
		{"container start", "Start a container"},
		{"container stop", "Stop a container (force)"},
		{"container shutdown", "Graceful container shutdown"},
		{"container suspend", "Suspend a container"},
		{"container resume", "Resume a suspended container"},
	}
	for _, v := range ctLifecycleReversible {
		ops = append(ops, OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
			RiskDimensions: []RiskDimension{RiskServiceDown},
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes:         []string{"table", "json", "yaml"},
			CapabilityInterface: "LifecycleProvider", HandlerFunc: "run" + toHandler(v.op),
		})
	}

	// --- container (lifecycle mutations - Tier 2: disruptive) ---
	ops = append(ops, OperationMeta{
		Path: "container reboot", Description: "Reboot a container",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
		RiskDimensions: []RiskDimension{RiskServiceDown},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "LifecycleProvider", HandlerFunc: "runCTReboot",
	})

	// --- container (config mutations) ---
	ops = append(ops, OperationMeta{
		Path: "container update", Description: "Update container configuration",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
		RiskDimensions: []RiskDimension{RiskServiceDown},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "ConfigProvider", HandlerFunc: "runCTUpdate",
	})

	// --- container (destructive mutations) ---
	ops = append(ops, OperationMeta{
		Path: "container delete", Description: "Delete a container (destructive)",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDestructive,
		RiskDimensions:      []RiskDimension{RiskDataLoss},
		RequiresTypeConfirm: true,
		Waitable:            true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "DeleteProvider", HandlerFunc: "runCTDelete",
	})

	// --- container (disruptive mutations) ---
	ctDisruptive := []struct {
		op, desc, capIface, handler string
		risks                       []RiskDimension
	}{
		{"container template", "Convert container to template", "TemplateProvider", "runCTTemplate", []RiskDimension{RiskDataLoss}},
		{"container migrate", "Migrate container to another node", "MigrationProvider", "runCTMigrate", []RiskDimension{RiskServiceDown}},
		{"container clone", "Clone a container", "CloneProvider", "runCTClone", nil},
	}
	for _, v := range ctDisruptive {
		ops = append(ops, OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
			RiskDimensions: v.risks,
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes:         []string{"table", "json", "yaml"},
			CapabilityInterface: v.capIface, HandlerFunc: v.handler,
		})
	}

	// --- container snapshot mutations ---
	ops = append(ops, OperationMeta{
		Path: "container snapshot create", Description: "Create a container snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierReversible,
		Waitable: true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runCTSnapshotCreate",
	})
	ops = append(ops, OperationMeta{
		Path: "container snapshot delete", Description: "Delete a container snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDestructive,
		RiskDimensions:      []RiskDimension{RiskDataLoss},
		RequiresTypeConfirm: true,
		Waitable:            true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runCTSnapshotDelete",
	})
	ops = append(ops, OperationMeta{
		Path: "container snapshot rollback", Description: "Rollback to a container snapshot",
		Inspection: false, Scope: ScopeGuest, SafetyTier: safety.TierDisruptive,
		RiskDimensions: []RiskDimension{RiskDataLoss, RiskServiceDown},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "SnapshotMutationProvider", HandlerFunc: "runCTSnapshotRollback",
	})

	// --- storage ---
	ops = append(ops, OperationMeta{
		Path: "storage list", Description: "List all storage pools",
		Inspection: true, Scope: ScopeStorage, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "StorageInspector", HandlerFunc: "runStorageList",
	})
	ops = append(ops, OperationMeta{
		Path: "storage show", Description: "Show storage details",
		Inspection: true, Scope: ScopeStorage, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "StorageInspector", HandlerFunc: "runStorageShow",
	})
	ops = append(ops, OperationMeta{
		Path: "storage content", Description: "List storage content",
		Inspection: true, Scope: ScopeStorage, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "StorageInspector", HandlerFunc: "runStorageContent",
	})
	ops = append(ops, OperationMeta{
		Path: "storage upload", Description: "Upload a file to storage",
		Inspection: false, Scope: ScopeStorage, SafetyTier: safety.TierDisruptive,
		Waitable: true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "StorageMutationProvider", HandlerFunc: "runStorageUpload",
	})
	ops = append(ops, OperationMeta{
		Path: "storage download", Description: "Download a volume from storage",
		Inspection: false, Scope: ScopeStorage, SafetyTier: safety.TierReversible,
		OutputModes:         []string{"table"},
		CapabilityInterface: "StorageMutationProvider", HandlerFunc: "runStorageDownload",
	})
	ops = append(ops, OperationMeta{
		Path: "storage delete", Description: "Delete a storage volume (destructive)",
		Inspection: false, Scope: ScopeStorage, SafetyTier: safety.TierDestructive,
		RiskDimensions:      []RiskDimension{RiskDataLoss},
		RequiresTypeConfirm: true,
		Waitable:            true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "StorageMutationProvider", HandlerFunc: "runStorageDelete",
	})

	// --- cluster ---
	ops = append(ops, OperationMeta{
		Path: "cluster status", Description: "Show cluster status",
		Inspection: true, Scope: ScopeCluster, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ClusterInspector", HandlerFunc: "runClusterStatus",
	})
	ops = append(ops, OperationMeta{
		Path: "cluster log", Description: "Show cluster log entries",
		Inspection: true, Scope: ScopeCluster, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ClusterLogProvider", HandlerFunc: "runClusterLog",
	})

	// --- event ---
	ops = append(ops, OperationMeta{
		Path: "event list", Description: "List cluster events",
		Inspection: true, Scope: ScopeCluster, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "EventInspector", HandlerFunc: "runEventList",
	})

	// --- log ---
	ops = append(ops, OperationMeta{
		Path: "log", Description: "Show node syslog",
		Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SyslogInspector", HandlerFunc: "runLog",
	})

	// --- doctor ---
	ops = append(ops, OperationMeta{
		Path: "doctor", Description: "Check system health",
		Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table"}, HandlerFunc: "runDoctor",
	})

	// --- task ---
	ops = append(ops, OperationMeta{
		Path: "task list", Description: "List all tasks for a node",
		Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "TaskInspector", HandlerFunc: "runTaskList",
	})
	ops = append(ops, OperationMeta{
		Path: "task show", Description: "Show task details",
		Inspection: true, Scope: ScopeNode, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "TaskInspector", HandlerFunc: "runTaskShow",
	})

	// --- backup ---
	ops = append(ops, OperationMeta{
		Path: "backup list", Description: "List backup tasks",
		Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "BackupInspector", HandlerFunc: "runBackupList",
	})
	ops = append(ops, OperationMeta{
		Path: "backup content", Description: "List backup content",
		Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "BackupProvider", HandlerFunc: "runBackupContent",
	})
	ops = append(ops, OperationMeta{
		Path: "backup create", Description: "Create a manual backup",
		Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
		Waitable: true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupCreate",
	})
	ops = append(ops, OperationMeta{
		Path: "backup restore", Description: "Restore VM from backup archive",
		Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
		RiskDimensions: []RiskDimension{RiskDataLoss},
		Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
		OutputModes:         []string{"table", "json", "yaml"},
		CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupRestore",
	})
	// backup job subcommands
	ops = append(ops, OperationMeta{
		Path: "backup job list", Description: "List backup job schedules",
		Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupJobList",
	})
	ops = append(ops, OperationMeta{
		Path: "backup job show", Description: "Show a backup schedule",
		Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupJobShow",
	})
	ops = append(ops, OperationMeta{
		Path: "backup job create", Description: "Create a backup schedule",
		Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
		OutputModes:         []string{"table"},
		CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupJobCreate",
	})
	ops = append(ops, OperationMeta{
		Path: "backup job update", Description: "Update a backup schedule",
		Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
		OutputModes:         []string{"table"},
		CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupJobUpdate",
	})
	ops = append(ops, OperationMeta{
		Path: "backup job delete", Description: "Delete a backup schedule (destructive)",
		Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDestructive,
		RequiresTypeConfirm: true,
		OutputModes:         []string{"table"},
		CapabilityInterface: "BackupMutationProvider", HandlerFunc: "runBackupJobDelete",
	})

	// --- firewall ---
	ops = append(ops, OperationMeta{
		Path: "firewall list", Description: "List firewall rules",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallInspector", HandlerFunc: "runFirewallList",
	})
	ops = append(ops, OperationMeta{
		Path: "firewall aliases", Description: "List firewall aliases",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallProvider", HandlerFunc: "runFirewallAliases",
	})
	ops = append(ops, OperationMeta{
		Path: "firewall ipsets", Description: "List firewall IP sets",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallProvider", HandlerFunc: "runFirewallIPSets",
	})
	ops = append(ops, OperationMeta{
		Path: "firewall security-groups", Description: "List firewall security groups",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallProvider", HandlerFunc: "runFirewallSecurityGroups",
	})
	ops = append(ops, OperationMeta{
		Path: "firewall node-rules", Description: "List node-level firewall rules",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallProvider", HandlerFunc: "runFirewallNodeRules",
	})
	ops = append(ops, OperationMeta{
		Path: "firewall vm-rules", Description: "List VM-level firewall rules",
		Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "FirewallProvider", HandlerFunc: "runFirewallVMRules",
	})
	// firewall mutations
	fwMutations := []struct {
		op, desc, capIface, handler string
		tier                        safety.Tier
		destructive                 bool
	}{
		{"firewall rule create", "Create a firewall rule", "FirewallMutationProvider", "runFirewallRuleCreate", safety.TierDisruptive, false},
		{"firewall rule update", "Update a firewall rule", "FirewallMutationProvider", "runFirewallRuleUpdate", safety.TierDisruptive, false},
		{"firewall rule delete", "Delete a firewall rule", "FirewallMutationProvider", "runFirewallRuleDelete", safety.TierDestructive, true},
		{"firewall alias create", "Create a firewall alias", "FirewallMutationProvider", "runFirewallAliasCreate", safety.TierDisruptive, false},
		{"firewall alias delete", "Delete a firewall alias", "FirewallMutationProvider", "runFirewallAliasDelete", safety.TierDestructive, true},
		{"firewall ipset create", "Create a firewall IP set", "FirewallMutationProvider", "runFirewallIPSetCreate", safety.TierDisruptive, false},
		{"firewall ipset entry add", "Add an IP set entry", "FirewallMutationProvider", "runFirewallIPSetEntryAdd", safety.TierDisruptive, false},
		{"firewall ipset entry remove", "Remove an IP set entry", "FirewallMutationProvider", "runFirewallIPSetEntryRemove", safety.TierDestructive, true},
		{"firewall ipset delete", "Delete a firewall IP set", "FirewallMutationProvider", "runFirewallIPSetDelete", safety.TierDestructive, true},
		{"firewall group create", "Create a security group", "FirewallMutationProvider", "runFirewallGroupCreate", safety.TierDisruptive, false},
		{"firewall group delete", "Delete a security group", "FirewallMutationProvider", "runFirewallGroupDelete", safety.TierDestructive, true},
		{"firewall options update", "Update firewall options", "FirewallMutationProvider", "runFirewallOptionsUpdate", safety.TierDisruptive, false},
	}
	for _, v := range fwMutations {
		meta := OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeFirewall, SafetyTier: v.tier,
			OutputModes:         []string{"table"},
			CapabilityInterface: v.capIface, HandlerFunc: v.handler,
		}
		if v.destructive {
			meta.RequiresTypeConfirm = true
		}
		ops = append(ops, meta)
	}

	// --- ha ---
	haOps := []OperationMeta{
		{Path: "ha list", Description: "List HA resources", Inspection: true, Scope: ScopeHA, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "HAInspector", HandlerFunc: "runHAList"},
		{Path: "ha groups", Description: "List HA groups", Inspection: true, Scope: ScopeHA, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "HAInspector", HandlerFunc: "runHAGroups"},
		{Path: "ha status", Description: "Show HA status", Inspection: true, Scope: ScopeHA, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "HAProvider", HandlerFunc: "runHAStatus"},
		{Path: "ha current", Description: "Show current HA resource state", Inspection: true, Scope: ScopeHA, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "HAProvider", HandlerFunc: "runHACurrent"},
	}
	ops = append(ops, haOps...)

	// --- sdn ---
	ops = append(ops, OperationMeta{
		Path: "sdn zones", Description: "List SDN zones",
		Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SDNProvider", HandlerFunc: "runSDNZones",
	})
	ops = append(ops, OperationMeta{
		Path: "sdn vnets", Description: "List SDN VNets",
		Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "SDNProvider", HandlerFunc: "runSDNVNets",
	})
	sdnMutations := []struct {
		op, desc, handler string
		destructive       bool
	}{
		{"sdn zone create", "Create an SDN zone", "runSDNZoneCreate", false},
		{"sdn zone delete", "Delete an SDN zone", "runSDNZoneDelete", true},
		{"sdn vnet create", "Create an SDN VNet", "runSDNVNetCreate", false},
		{"sdn vnet delete", "Delete an SDN VNet", "runSDNVNetDelete", true},
		{"sdn subnet create", "Create an SDN subnet", "runSDNSubnetCreate", false},
		{"sdn subnet delete", "Delete an SDN subnet", "runSDNSubnetDelete", true},
		{"sdn controller create", "Create an SDN controller", "runSDNControllerCreate", false},
		{"sdn controller delete", "Delete an SDN controller", "runSDNControllerDelete", true},
	}
	for _, v := range sdnMutations {
		tier := safety.TierDisruptive
		if v.destructive {
			tier = safety.TierDestructive
		}
		meta := OperationMeta{
			Path: v.op, Description: v.desc,
			Inspection: false, Scope: ScopeSDN, SafetyTier: tier,
			OutputModes:         []string{"table"},
			CapabilityInterface: "SDNMutationProvider", HandlerFunc: v.handler,
		}
		if v.destructive {
			meta.RequiresTypeConfirm = true
		}
		ops = append(ops, meta)
	}

	// --- pools ---
	ops = append(ops, OperationMeta{
		Path: "pools list", Description: "List all resource pools",
		Inspection: true, Scope: ScopeCluster, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PoolProvider", HandlerFunc: "runPoolsList",
	})

	// --- network ---
	ops = append(ops, OperationMeta{
		Path: "network show", Description: "Show node network interfaces",
		Inspection: true, Scope: ScopeNetwork, SafetyTier: safety.TierObservation,
		OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "NodeDetailProvider", HandlerFunc: "runNetworkShow",
	})
	ops = append(ops, OperationMeta{
		Path: "network apply", Description: "Apply (reload) pending network configuration",
		Inspection: false, Scope: ScopeNetwork, SafetyTier: safety.TierDisruptive,
		RiskDimensions:      []RiskDimension{RiskNetworkLock, RiskServiceDown},
		ProducesUPID:        true,
		OutputModes:         []string{"table"},
		CapabilityInterface: "NetworkMutationProvider", HandlerFunc: "runNetworkApply",
	})
	ops = append(ops, OperationMeta{
		Path: "network revert", Description: "Revert pending network changes",
		Inspection: false, Scope: ScopeNetwork, SafetyTier: safety.TierDisruptive,
		RiskDimensions:      []RiskDimension{RiskNetworkLock},
		OutputModes:         []string{"table"},
		CapabilityInterface: "NetworkMutationProvider", HandlerFunc: "runNetworkRevert",
	})

	// --- access ---
	accessInspect := []OperationMeta{
		{Path: "access users list", Description: "List users", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessUsersList"},
		{Path: "access groups list", Description: "List groups", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessGroupsList"},
		{Path: "access roles list", Description: "List roles", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessRolesList"},
		{Path: "access acl list", Description: "List ACL entries", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessACLList"},
		{Path: "access domains list", Description: "List auth domains", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessDomainsList"},
		{Path: "access tokens list", Description: "List API tokens for a user", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessTokensList"},
	}
	ops = append(ops, accessInspect...)

	accessMutations := []OperationMeta{
		{Path: "access user create", Description: "Create a user", Inspection: false, Scope: ScopeAccess, SafetyTier: safety.TierSecurityAdmin, RequiresExpert: true, SecuritySensitivity: SecIdentity, OutputModes: []string{"table"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessUserCreate"},
		{Path: "access user delete", Description: "Delete a user", Inspection: false, Scope: ScopeAccess, SafetyTier: safety.TierSecurityAdmin, RequiresExpert: true, SecuritySensitivity: SecIdentity, OutputModes: []string{"table"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessUserDelete"},
		{Path: "access acl add", Description: "Add an ACL entry", Inspection: false, Scope: ScopeAccess, SafetyTier: safety.TierSecurityAdmin, RequiresExpert: true, SecuritySensitivity: SecAccess, OutputModes: []string{"table"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessACLAdd"},
	}
	ops = append(ops, accessMutations...)

	// --- ceph ---
	cephInspect := []OperationMeta{
		{Path: "ceph status", Description: "Show Ceph cluster status", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephProvider", HandlerFunc: "runCephStatus"},
		{Path: "ceph osd list", Description: "List Ceph OSDs", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephProvider", HandlerFunc: "runCephOSDList"},
		{Path: "ceph mon list", Description: "List Ceph monitors", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephProvider", HandlerFunc: "runCephMONList"},
		{Path: "ceph pool list", Description: "List Ceph pools", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephProvider", HandlerFunc: "runCephPoolList"},
	}
	ops = append(ops, cephInspect...)

	cephMutations := []OperationMeta{
		{Path: "ceph osd create", Description: "Create a Ceph OSD", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierDisruptive, RiskDimensions: []RiskDimension{RiskDataLoss}, Waitable: true, ProducesUPID: true, UsesOperationResult: true, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephOSDCreate"},
		{Path: "ceph osd out", Description: "Mark Ceph OSD out", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierDisruptive, RiskDimensions: []RiskDimension{RiskServiceDown}, OutputModes: []string{"table"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephOSDOut"},
		{Path: "ceph osd in", Description: "Mark Ceph OSD in", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierReversible, OutputModes: []string{"table"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephOSDIn"},
		{Path: "ceph osd destroy", Description: "Destroy a Ceph OSD", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierDestructive, RiskDimensions: []RiskDimension{RiskDataLoss}, RequiresTypeConfirm: true, Waitable: true, ProducesUPID: true, UsesOperationResult: true, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephOSDDestroy"},
		{Path: "ceph pool create", Description: "Create a Ceph pool", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierDisruptive, Waitable: true, ProducesUPID: true, UsesOperationResult: true, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephPoolCreate"},
		{Path: "ceph pool destroy", Description: "Destroy a Ceph pool", Inspection: false, Scope: ScopeCeph, SafetyTier: safety.TierDestructive, RiskDimensions: []RiskDimension{RiskDataLoss}, RequiresTypeConfirm: true, Waitable: true, ProducesUPID: true, UsesOperationResult: true, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "CephMutationProvider", HandlerFunc: "runCephPoolDestroy"},
	}
	ops = append(ops, cephMutations...)

	// --- replication ---
	replOps := []OperationMeta{
		{Path: "replication list", Description: "List replication jobs", Inspection: true, Scope: ScopeRepl, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationList"},
		{Path: "replication show", Description: "Show replication job details", Inspection: true, Scope: ScopeRepl, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationShow"},
		{Path: "replication create", Description: "Create a replication job", Inspection: false, Scope: ScopeRepl, SafetyTier: safety.TierDisruptive, OutputModes: []string{"table"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationCreate"},
		{Path: "replication update", Description: "Update a replication job", Inspection: false, Scope: ScopeRepl, SafetyTier: safety.TierDisruptive, OutputModes: []string{"table"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationUpdate"},
		{Path: "replication delete", Description: "Delete a replication job", Inspection: false, Scope: ScopeRepl, SafetyTier: safety.TierDestructive, RequiresTypeConfirm: true, OutputModes: []string{"table"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationDelete"},
		{Path: "replication schedule", Description: "Schedule replication job now", Inspection: false, Scope: ScopeRepl, SafetyTier: safety.TierReversible, OutputModes: []string{"table"}, CapabilityInterface: "ReplicationProvider", HandlerFunc: "runReplicationSchedule"},
	}
	ops = append(ops, replOps...)

	// --- Dispatch/routing commands ---
	// These are commands that route to sub-commands. They have a run handler
	// but delegate to sub-operations. Their safety tier is Observation because
	// the dispatch itself does not mutate state; the sub-commands handle safety.
	dispatchOps := []OperationMeta{
		{Path: "vm snapshot", Description: "Manage VM snapshots (routing)", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runVMSnapshotDispatch"},
		{Path: "vm disk", Description: "Manage VM disks (routing)", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runVMDiskDispatch"},
		{Path: "container snapshot", Description: "Manage container snapshots (routing)", Inspection: true, Scope: ScopeGuest, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runCTSnapshotDispatch"},
		{Path: "firewall rule", Description: "Manage firewall rules (routing)", Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runFirewallRuleDispatch"},
		{Path: "firewall alias", Description: "Manage firewall aliases (routing)", Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runFirewallAliasDispatch"},
		{Path: "firewall ipset", Description: "Manage firewall IP sets (routing)", Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runFirewallIPSetDispatch"},
		{Path: "firewall group", Description: "Manage firewall security groups (routing)", Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runFirewallGroupDispatch"},
		{Path: "firewall options", Description: "Manage firewall options (routing)", Inspection: true, Scope: ScopeFirewall, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runFirewallOptionsDispatch"},
		{Path: "backup job", Description: "Manage backup job schedules (routing)", Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runBackupJobDispatch"},
		{Path: "sdn zone", Description: "Manage SDN zones (routing)", Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runSDNZoneDispatch"},
		{Path: "sdn vnet", Description: "Manage SDN VNets (routing)", Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runSDNVNetDispatch"},
		{Path: "sdn subnet", Description: "Manage SDN subnets (routing)", Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runSDNSubnetDispatch"},
		{Path: "sdn controller", Description: "Manage SDN controllers (routing)", Inspection: true, Scope: ScopeSDN, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runSDNControllerDispatch"},
		{Path: "ceph osd", Description: "Manage Ceph OSDs (routing)", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runCephOSDDispatch"},
		{Path: "ceph mon", Description: "Manage Ceph monitors (routing)", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runCephMonDispatch"},
		{Path: "ceph pool", Description: "Manage Ceph pools (routing)", Inspection: true, Scope: ScopeCeph, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runCephPoolDispatch"},
		{Path: "access user", Description: "Manage individual users (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runAccessUserDispatch"},
		{Path: "access users", Description: "List users (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessUsersDispatch"},
		{Path: "access groups", Description: "List groups (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessGroupsDispatch"},
		{Path: "access roles", Description: "List roles (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessRolesDispatch"},
		{Path: "access acl", Description: "Manage ACL entries (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table"}, HandlerFunc: "runAccessACLDispatch"},
		{Path: "access domains", Description: "List auth domains (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessDomainsDispatch"},
		{Path: "access tokens", Description: "List API tokens (routing)", Inspection: true, Scope: ScopeAccess, SafetyTier: safety.TierObservation, OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "AccessProvider", HandlerFunc: "runAccessTokensDispatch"},
		// "version" is both a leaf command (show version) and a parent (compare, parse).
		// Already registered above, no dispatch entry needed since "version" itself shows version info.
	}
	ops = append(ops, dispatchOps...)

	// --- maintenance (fleet, read-only in phase 5) ---
	maintOps := []OperationMeta{
		{Path: "maintenance inventory", Description: "List enrolled maintenance hosts",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runMaintenanceInventory"},
		{Path: "maintenance status", Description: "Read-only maintenance preflight status",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runMaintenanceStatus"},
		{Path: "maintenance plan", Description: "Create an immutable maintenance plan (makes no changes)",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runMaintenancePlan"},
	}
	ops = append(ops, maintOps...)

	// --- environment (unified PVE/PBS health) ---
	envOps := []OperationMeta{
		{Path: "environment list", Description: "List configured environments",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runEnvironmentList"},
		{Path: "environment health", Description: "Check environment infrastructure health",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runEnvironmentHealth"},
		{Path: "environment backup-health", Description: "Check environment backup health and guest coverage",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, HandlerFunc: "runEnvironmentBackupHealth"},
	}
	ops = append(ops, envOps...)

	// --- pbs (Proxmox Backup Server, read-only) ---
	pbsOps := []OperationMeta{
		{Path: "pbs status", Description: "Show PBS host status",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSystemInspector", HandlerFunc: "runPBSStatus"},
		{Path: "pbs version", Description: "Show PBS server version",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSystemInspector", HandlerFunc: "runPBSVersion"},
		{Path: "pbs subscription", Description: "Show PBS subscription status",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSystemInspector", HandlerFunc: "runPBSSubscription"},
		{Path: "pbs certificates", Description: "List PBS certificates",
			Inspection: true, Scope: ScopeSystem, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSystemInspector", HandlerFunc: "runPBSCertificates"},
		{Path: "pbs datastore", Description: "Inspect PBS datastores (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSDatastoreDispatch"},
		{Path: "pbs datastore list", Description: "List PBS datastores",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSDatastoreInspector", HandlerFunc: "runPBSDatastoreList"},
		{Path: "pbs datastore show", Description: "Show PBS datastore configuration and usage",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSDatastoreInspector", HandlerFunc: "runPBSDatastoreShow"},
		{Path: "pbs snapshot", Description: "Inspect PBS backup snapshots (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSSnapshotDispatch"},
		{Path: "pbs snapshot list", Description: "List PBS backup snapshots in a datastore",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSnapshotInspector", HandlerFunc: "runPBSSnapshotList"},
		{Path: "pbs task", Description: "Inspect PBS tasks (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSTaskDispatch"},
		{Path: "pbs task list", Description: "List PBS tasks",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSTaskInspector", HandlerFunc: "runPBSTaskList"},
		{Path: "pbs task show", Description: "Show PBS task details",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSTaskInspector", HandlerFunc: "runPBSTaskShow"},
		{Path: "pbs task log", Description: "Show PBS task log",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSTaskInspector", HandlerFunc: "runPBSTaskLog"},
		{Path: "pbs verify", Description: "Inspect PBS verification jobs (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSVerifyDispatch"},
		{Path: "pbs verify list", Description: "List PBS verification jobs",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSJobInspector", HandlerFunc: "runPBSVerifyList"},
		{Path: "pbs prune", Description: "Inspect PBS prune jobs (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSPruneDispatch"},
		{Path: "pbs prune list", Description: "List PBS prune jobs",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSJobInspector", HandlerFunc: "runPBSPruneList"},
		{Path: "pbs sync", Description: "Inspect PBS sync jobs (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSSyncDispatch"},
		{Path: "pbs sync list", Description: "List PBS sync jobs",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSJobInspector", HandlerFunc: "runPBSSyncList"},
		{Path: "pbs garbage-collection", Description: "Inspect PBS garbage collection (routing)",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table"}, HandlerFunc: "runPBSGCDispatch"},
		{Path: "pbs garbage-collection status", Description: "Show PBS garbage-collection status",
			Inspection: true, Scope: ScopeBackup, SafetyTier: safety.TierObservation,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSGCInspector", HandlerFunc: "runPBSGCStatus"},
		{Path: "pbs verify run", Description: "Run a PBS verification job or verify a datastore",
			Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierReversible,
			Waitable: true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSVerifyRunner", HandlerFunc: "runPBSVerifyRun"},
		{Path: "pbs sync run", Description: "Run a PBS sync job",
			Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
			RiskDimensions: []RiskDimension{RiskDataLoss},
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSSyncRunner", HandlerFunc: "runPBSSyncRun"},
		{Path: "pbs prune run", Description: "Run a PBS prune job (removes snapshots)",
			Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDestructive,
			RiskDimensions: []RiskDimension{RiskDataLoss}, RequiresTypeConfirm: true,
			Waitable: true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSPruneRunner", HandlerFunc: "runPBSPruneRun"},
		{Path: "pbs garbage-collection run", Description: "Run PBS garbage collection on a datastore",
			Inspection: false, Scope: ScopeBackup, SafetyTier: safety.TierDisruptive,
			RiskDimensions: []RiskDimension{RiskDataLoss},
			Waitable:       true, ProducesUPID: true, UsesOperationResult: true,
			OutputModes: []string{"table", "json", "yaml"}, CapabilityInterface: "PBSGCRunner", HandlerFunc: "runPBSGCRun"},
	}
	ops = append(ops, pbsOps...)

	// Sort by path for deterministic output.
	sort.Slice(ops, func(i, j int) bool { return ops[i].Path < ops[j].Path })
	return ops
}

// Operations returns the full canonical operation registry.
func Operations() []OperationMeta {
	return operationRegistry
}

// LookupOperation finds an operation by its full command path.
// Returns nil if no operation with the given path exists.
func LookupOperation(path string) *OperationMeta {
	for i := range operationRegistry {
		if operationRegistry[i].Path == path {
			return &operationRegistry[i]
		}
	}
	return nil
}

// MutationOperations returns all operations that are not inspection-only.
func MutationOperations() []OperationMeta {
	var result []OperationMeta
	for _, op := range operationRegistry {
		if !op.Inspection {
			result = append(result, op)
		}
	}
	return result
}

// InspectionOperations returns all read-only operations.
func InspectionOperations() []OperationMeta {
	var result []OperationMeta
	for _, op := range operationRegistry {
		if op.Inspection {
			result = append(result, op)
		}
	}
	return result
}

// OperationsByTier returns operations at a specific safety tier.
func OperationsByTier(tier safety.Tier) []OperationMeta {
	var result []OperationMeta
	for _, op := range operationRegistry {
		if op.SafetyTier == tier {
			result = append(result, op)
		}
	}
	return result
}

// ValidateRegistry checks the operation registry for consistency:
// - Every command-tree leaf has a registry entry (or is a known dispatch).
// - Every registry entry exists in the command tree or is a known dispatch sub-op.
// - No inspection operation has a safety tier above TierObservation.
// - Every non-inspection operation has a valid safety tier.
// - No inspection operation claims to produce a UPID.
func ValidateRegistry() []error {
	var errs []error

	// Collect all registered command paths from the tree.
	treePaths := collectCommandPaths(commands, "")
	treePathSet := make(map[string]bool)
	for _, p := range treePaths {
		treePathSet[p] = true
	}

	// Collect registry paths.
	regEntrySet := make(map[string]bool)
	for _, op := range operationRegistry {
		regEntrySet[op.Path] = true
	}

	// Build the set of all dispatch sub-ops (for cross-referencing).
	dispatchSubOps := make(map[string]bool)
	for _, subs := range knownDispatchCommands {
		for _, sub := range subs {
			dispatchSubOps[sub] = true
		}
	}

	// Check: every tree path has a registry entry (or is a dispatch parent for
	// whose sub-ops we have entries).
	for _, p := range treePaths {
		if regEntrySet[p] {
			continue
		}
		// Allow dispatch parents if their sub-ops are registered.
		if subs, ok := knownDispatchCommands[p]; ok {
			missing := false
			for _, sub := range subs {
				if !regEntrySet[sub] {
					missing = true
					break
				}
			}
			if !missing {
				continue
			}
		}
		errs = append(errs, fmt.Errorf("command path %q registered in command tree but missing from operation registry", p))
	}

	// Check: every registry entry exists in the tree or is a known dispatch sub-op.
	for _, op := range operationRegistry {
		if treePathSet[op.Path] || dispatchSubOps[op.Path] {
			continue
		}
		errs = append(errs, fmt.Errorf("registry entry %q not found in command tree or dispatch sub-ops", op.Path))
	}

	// Check consistency of each entry.
	for _, op := range operationRegistry {
		if op.Inspection {
			if op.SafetyTier != safety.TierObservation {
				errs = append(errs, fmt.Errorf("%q is inspection but has tier %s (want observation)", op.Path, op.SafetyTier))
			}
			if op.ProducesUPID {
				errs = append(errs, fmt.Errorf("%q is inspection but claims to produce UPID", op.Path))
			}
			if op.RequiresTypeConfirm {
				errs = append(errs, fmt.Errorf("%q is inspection but requires type confirm", op.Path))
			}
			if op.RequiresExpert {
				errs = append(errs, fmt.Errorf("%q is inspection but requires expert", op.Path))
			}
		} else {
			if op.SafetyTier == safety.TierObservation {
				errs = append(errs, fmt.Errorf("%q is mutation but has tier observation", op.Path))
			}
		}

		if op.RequiresTypeConfirm && op.SafetyTier < safety.TierDestructive {
			errs = append(errs, fmt.Errorf("%q requires type confirm but tier is %s (should be destructive+)", op.Path, op.SafetyTier))
		}

		if op.RequiresExpert && op.SafetyTier != safety.TierSecurityAdmin {
			errs = append(errs, fmt.Errorf("%q requires expert but tier is %s (should be security_admin)", op.Path, op.SafetyTier))
		}
	}

	return errs
}

// collectCommandPaths recursively walks the command tree and returns all
// reachable command paths. A command with both run and sub (e.g., "version")
// is reachable directly AND via its sub-commands.
func collectCommandPaths(cmds map[string]*command, prefix string) []string {
	var paths []string
	for _, cmd := range cmds {
		fullPath := cmd.name
		if prefix != "" {
			fullPath = prefix + " " + cmd.name
		}
		if cmd.sub != nil {
			subs := collectCommandPaths(cmd.sub, fullPath)
			paths = append(paths, subs...)
		}
		if cmd.run != nil {
			// This command has a handler; it's reachable directly.
			paths = append(paths, fullPath)
		}
	}
	return paths
}

// knownDispatchCommands maps dispatch-command paths to their known sub-commands.
// These are commands that have a handler (run) but internally route to sub-ops.
// The registry includes entries for both the dispatch and its sub-ops.
var knownDispatchCommands = map[string][]string{
	"vm snapshot":        {"vm snapshot create", "vm snapshot delete", "vm snapshot rollback"},
	"container snapshot": {"container snapshot create", "container snapshot delete", "container snapshot rollback"},
	"vm disk":            {"vm disk resize", "vm disk move"},
	"firewall rule":      {"firewall rule create", "firewall rule update", "firewall rule delete"},
	"firewall alias":     {"firewall alias create", "firewall alias delete"},
	"firewall ipset":     {"firewall ipset create", "firewall ipset entry add", "firewall ipset entry remove", "firewall ipset delete"},
	"firewall group":     {"firewall group create", "firewall group delete"},
	"firewall options":   {"firewall options update"},
	"backup job":         {"backup job list", "backup job show", "backup job create", "backup job update", "backup job delete"},
	"sdn zone":           {"sdn zone create", "sdn zone delete"},
	"sdn vnet":           {"sdn vnet create", "sdn vnet delete"},
	"sdn subnet":         {"sdn subnet create", "sdn subnet delete"},
	"sdn controller":     {"sdn controller create", "sdn controller delete"},
	"ceph osd":           {"ceph osd list", "ceph osd create", "ceph osd out", "ceph osd in", "ceph osd destroy"},
	"ceph mon":           {"ceph mon list"},
	"ceph pool":          {"ceph pool list", "ceph pool create", "ceph pool destroy"},
	"access user":        {"access user create", "access user delete"},
	"access users":       {"access users list"},
	"access groups":      {"access groups list"},
	"access roles":       {"access roles list"},
	"access acl":         {"access acl list", "access acl add"},
	"access domains":     {"access domains list"},
	"access tokens":      {"access tokens list"},

	// PBS dispatchers.
	"pbs datastore":          {"pbs datastore list", "pbs datastore show"},
	"pbs snapshot":           {"pbs snapshot list"},
	"pbs task":               {"pbs task list", "pbs task show", "pbs task log"},
	"pbs verify":             {"pbs verify list", "pbs verify run"},
	"pbs prune":              {"pbs prune list", "pbs prune run"},
	"pbs sync":               {"pbs sync list", "pbs sync run"},
	"pbs garbage-collection": {"pbs garbage-collection status", "pbs garbage-collection run"},
}

// toHandler converts an operation path like "vm start" to a handler name like "VMStart".
func toHandler(path string) string {
	parts := strings.Split(path, " ")
	var result string
	for _, p := range parts {
		if len(p) > 0 {
			result += strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return result
}
