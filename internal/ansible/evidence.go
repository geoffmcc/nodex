package ansible

// EvidenceOutcomeUsable reports whether one task outcome can satisfy its
// operation contract. A nonzero return code is accepted only for diagnostic
// container checks whose playbooks deliberately use it as evidence.
func EvidenceOutcomeUsable(operation string, outcome TaskOutcome) bool {
	if outcome.Failed || outcome.Unreachable {
		return false
	}
	if outcome.Skipped {
		return operation == "apply-security-updates" && outcome.EvidenceID == HostEvidenceUpdate
	}
	if outcome.RC == nil {
		return true
	}
	rc := *outcome.RC
	if outcome.EvidenceID == ContainerEvidenceDpkg {
		return true
	}
	if outcome.EvidenceID == ContainerEvidenceReboot {
		return rc == 0 || rc == 1
	}
	return rc == 0
}

// EvidenceSchemaVersion identifies the machine-readable callback result shape.
const EvidenceSchemaVersion = 1

// EvidenceContract is deliberately separate from Ansible task names. Task
// names are presentation text and are not a trustworthy integration key.
const EvidenceContract = "nodex.ansible.task-results.v1"

// Stable evidence IDs used by the embedded container playbooks. These values
// are the contract consumed by the container update workflow.
const (
	HostEvidenceConnectivity = "host.connectivity"
	HostEvidenceDebian       = "host.os.debian"
	HostEvidenceRefresh      = "host.package.refresh"
	HostEvidencePackages     = "host.packages"
	HostEvidenceSimulation   = "host.upgrade_simulation"
	HostEvidenceReboot       = "host.reboot_required"
	HostEvidenceFailedUnits  = "host.failed_units"
	HostEvidenceRoot         = "host.root_usage"
	HostEvidenceUpdate       = "host.update"
	HostEvidenceUpdateReport = "host.update_report"

	ContainerEvidenceStatus     = "container.status"
	ContainerEvidenceAPT        = "container.apt"
	ContainerEvidenceRefresh    = "container.refresh"
	ContainerEvidencePackages   = "container.packages"
	ContainerEvidenceSimulation = "container.simulation"
	ContainerEvidenceDpkg       = "container.dpkg"
	ContainerEvidenceReboot     = "container.reboot"
	ContainerEvidenceRoot       = "container.root"
	ContainerEvidenceUpdate     = "container.update"
)

var hostCheckEvidence = []string{
	HostEvidenceDebian,
	HostEvidenceRefresh,
	HostEvidencePackages,
	HostEvidenceSimulation,
	HostEvidenceReboot,
	HostEvidenceFailedUnits,
	HostEvidenceRoot,
}

var hostVerifyEvidence = []string{
	HostEvidenceConnectivity,
	HostEvidenceReboot,
	HostEvidenceFailedUnits,
	HostEvidenceRoot,
}

var hostUpdateEvidence = []string{
	HostEvidenceDebian,
	HostEvidenceRefresh,
	HostEvidenceUpdate,
	HostEvidenceUpdateReport,
}

var containerPreflightEvidence = []string{
	ContainerEvidenceStatus,
	ContainerEvidenceAPT,
	ContainerEvidenceRefresh,
	ContainerEvidencePackages,
	ContainerEvidenceSimulation,
	ContainerEvidenceDpkg,
	ContainerEvidenceReboot,
	ContainerEvidenceRoot,
}

var containerApplyEvidence = []string{
	ContainerEvidenceStatus,
	ContainerEvidenceAPT,
	ContainerEvidenceRefresh,
	ContainerEvidenceSimulation,
	ContainerEvidenceUpdate,
}

var containerVerifyEvidence = []string{
	ContainerEvidenceStatus,
	ContainerEvidenceAPT,
	ContainerEvidencePackages,
	ContainerEvidenceSimulation,
	ContainerEvidenceDpkg,
	ContainerEvidenceReboot,
	ContainerEvidenceRoot,
}

// RequiredEvidenceIDs returns an immutable copy of an operation's evidence
// contract so callers cannot alter the registry through a returned slice.
func (o Operation) RequiredEvidenceIDs() []string {
	return append([]string(nil), o.RequiredEvidence...)
}
