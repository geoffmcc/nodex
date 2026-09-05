package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

const containerUpdatePolicy = "approved-full-upgrade"

const (
	containerStatusTask   = "Verify LXC guest is running"
	containerAPTTask      = "Verify LXC guest uses APT"
	containerPackagesTask = "List LXC upgradable packages"
	containerSimTask      = "Simulate LXC full upgrade"
	containerDpkgTask     = "Check LXC package database"
	containerRebootTask   = "Check LXC reboot-required marker"
	containerRootTask     = "Report LXC root filesystem usage"
)

// runContainerOSAnsible is the only execution seam for LXC guest updates.
// The PVE host is the Ansible target; the embedded playbook performs the fixed
// pct argument vector inside that host.
var runContainerOSAnsible = func(ctx context.Context, operation string, host ansible.HostSpec, vmid int) (*ansible.RunResult, error) {
	det, err := ansible.Detect(ctx)
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("container OS update requires Ansible: %w", err), app.ExitIncompatibility)
	}
	return (&ansible.Runner{Exe: det.Path}).Run(ctx, ansible.RunRequest{
		Operation:     operation,
		Hosts:         []ansible.HostSpec{host},
		ContainerVMID: vmid,
	})
}

type containerUpdateState struct {
	PendingUpdates   []string
	DpkgIssues       bool
	RebootRequired   bool
	RootUsage        string
	EvidenceComplete bool
}

func parseContainerOSUpdateArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--policy %s is required", containerUpdatePolicy)
	}
	policy := ""
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		switch arg {
		case "--policy":
			if len(args) == 0 {
				return fmt.Errorf("--policy requires a value")
			}
			value := args[0]
			args = args[1:]
			if strings.HasPrefix(value, "--") {
				return fmt.Errorf("--policy requires a value")
			}
			policy = value
		default:
			return fmt.Errorf("unknown argument %q", arg)
		}
	}
	if policy != containerUpdatePolicy {
		return fmt.Errorf("unsupported container update policy %q (only %q is currently supported)", policy, containerUpdatePolicy)
	}
	return nil
}

func findPVEInventoryHost(cfg *config.Config, profileName, node string) (string, config.InventoryHost, error) {
	if cfg.Inventory == nil {
		return "", config.InventoryHost{}, fmt.Errorf("no inventory configured; an enrolled PVE host is required")
	}
	if node == "" {
		return "", config.InventoryHost{}, fmt.Errorf("a PVE node is required to select the pct execution host")
	}
	var matches []struct {
		name string
		host config.InventoryHost
	}
	for name, host := range cfg.Inventory.Hosts {
		if host.Role == config.RolePVE && host.PVEProfile == profileName && host.PVENode == node {
			matches = append(matches, struct {
				name string
				host config.InventoryHost
			}{name: name, host: host})
		}
	}
	if len(matches) == 0 {
		return "", config.InventoryHost{}, fmt.Errorf("profile %q has no enrolled PVE host for node %q pct execution", profileName, node)
	}
	if len(matches) != 1 {
		return "", config.InventoryHost{}, fmt.Errorf("profile %q has %d enrolled PVE hosts for node %q; pct execution target is ambiguous", profileName, len(matches), node)
	}
	return matches[0].name, matches[0].host, nil
}

func interpretContainerOSUpdate(result *ansible.RunResult, hostName string) containerUpdateState {
	state := containerUpdateState{}
	if result == nil || !result.Success {
		return state
	}
	seen := map[string]bool{}
	usable := map[string]bool{}
	for _, outcome := range result.TaskOutcomes[hostName] {
		seen[outcome.Task] = true
		usable[outcome.Task] = containerTaskUsable(outcome)
		switch outcome.Task {
		case containerPackagesTask:
			state.PendingUpdates = parseContainerPackages(outcome.StdoutLines)
		case containerDpkgTask:
			state.DpkgIssues = (outcome.RC != nil && *outcome.RC != 0) || len(nonEmptyLines(outcome.StdoutLines)) > 0
		case containerRebootTask:
			if outcome.RC != nil {
				state.RebootRequired = *outcome.RC == 0
			} else {
				state.RebootRequired = len(nonEmptyLines(outcome.StdoutLines)) > 0
			}
		case containerRootTask:
			state.RootUsage = parseContainerRootUsage(outcome.StdoutLines)
		}
	}
	state.EvidenceComplete = true
	for _, task := range []string{containerStatusTask, containerAPTTask, containerPackagesTask, containerSimTask, containerDpkgTask, containerRebootTask, containerRootTask} {
		if !seen[task] || !usable[task] {
			state.EvidenceComplete = false
			break
		}
	}
	if state.RootUsage == "" {
		state.EvidenceComplete = false
	}
	return state
}

func containerTaskUsable(outcome ansible.TaskOutcome) bool {
	if outcome.Failed || outcome.Skipped || outcome.Unreachable {
		return false
	}
	if outcome.RC == nil {
		return false
	}
	rc := *outcome.RC
	// `stat` returns 1 when the reboot marker is absent. The package audit also
	// uses its return code as diagnostic evidence, so its non-zero result is
	// handled by DpkgIssues instead of being discarded as an execution error.
	if outcome.Task == containerDpkgTask {
		return true
	}
	if outcome.Task == containerRebootTask {
		return rc == 0 || rc == 1
	}
	return rc == 0
}

func missingContainerUpdateEvidence(result *ansible.RunResult, hostName string) []string {
	outcomes := map[string]ansible.TaskOutcome{}
	if result != nil {
		for _, outcome := range result.TaskOutcomes[hostName] {
			outcomes[outcome.Task] = outcome
		}
	}
	required := []string{containerStatusTask, containerAPTTask, containerPackagesTask, containerSimTask, containerDpkgTask, containerRebootTask, containerRootTask}
	missing := make([]string, 0)
	for _, task := range required {
		outcome, ok := outcomes[task]
		if !ok {
			missing = append(missing, task)
			continue
		}
		if !containerTaskUsable(outcome) {
			missing = append(missing, task+" (unusable)")
			continue
		}
		if task == containerRootTask && parseContainerRootUsage(outcome.StdoutLines) == "" {
			missing = append(missing, task+" (missing root usage)")
		}
	}
	return missing
}

func parseContainerPackages(lines []string) []string {
	packages := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") || strings.HasPrefix(line, "WARNING") {
			continue
		}
		if idx := strings.IndexByte(line, '/'); idx > 0 {
			packages = append(packages, line[:idx])
		}
	}
	return packages
}

func parseContainerRootUsage(lines []string) string {
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 6 && fields[5] == "/" {
			return fields[4]
		}
	}
	return ""
}

func nonEmptyLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

func runContainerOSUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex container os-update <node>/<vmid> --policy %s", containerUpdatePolicy)
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if err := parseContainerOSUpdateArgs(args[1:]); err != nil {
		return app.NewExitError(fmt.Errorf("%w: %w", usage, err), app.ExitUsage)
	}

	profileName, err := resolveProfileName(cmdCtx)
	if err != nil {
		return err
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	hostName, inventoryHost, err := findPVEInventoryHost(cfg, profileName, node)
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, profileName)
	if err != nil {
		return err
	}
	defer cleanup()
	ci, err := requireContainerInspector(prov)
	if err != nil {
		return err
	}
	containers, err := ci.Containers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	target := fmt.Sprintf("%s/%d", node, vmid)
	container, ok := findContainer(containers, target)
	if !ok {
		return app.NewExitError(fmt.Errorf("container %q not found", target), app.ExitNotFound)
	}
	if container.Status != "running" {
		return app.NewExitError(fmt.Errorf("container %s is %s; it must be running for pct OS update", target, container.Status), app.ExitValidationError)
	}

	host := ansible.HostSpec{
		Name:           hostName,
		Address:        inventoryHost.Address,
		Port:           inventoryHost.SSHPort,
		User:           inventoryHost.SSHUser,
		KeyFile:        inventoryHost.SSHKeyFile,
		KnownHostsFile: inventoryHost.KnownHostsFile,
	}
	preflight, err := runContainerOSAnsible(ctx, "check-container-updates", host, vmid)
	if err != nil {
		return err
	}
	before := interpretContainerOSUpdate(preflight, hostName)
	if !before.EvidenceComplete {
		return app.NewExitError(fmt.Errorf("container %s preflight was incomplete; missing or unusable task evidence: %s", target, strings.Join(missingContainerUpdateEvidence(preflight, hostName), ", ")), app.ExitPartialFailure)
	}
	if !preflight.Success {
		return app.NewExitError(fmt.Errorf("container %s preflight failed", target), app.ExitPartialFailure)
	}
	if before.DpkgIssues {
		return app.NewExitError(fmt.Errorf("container %s has package database issues; update refused", target), app.ExitValidationError)
	}
	if len(before.PendingUpdates) == 0 {
		return writeContainerUpdateResult(cmdCtx, prov, profileName, target, false, false, before, before)
	}

	if err := checkDisruptive(cmdCtx, fmt.Sprintf("update LXC guest %s using approved full upgrade", target)); err != nil {
		return err
	}
	applyResult, err := runContainerOSAnsible(ctx, "apply-container-updates", host, vmid)
	if err != nil {
		return app.NewExitError(fmt.Errorf("container %s update outcome is unknown; do not retry until a fresh preflight completes: %w", target, err), app.ExitAmbiguousOutcome)
	}
	if !applyResult.Success {
		if err := writeContainerUpdateResult(cmdCtx, prov, profileName, target, true, false, before, before); err != nil {
			return app.NewExitError(fmt.Errorf("container %s update outcome is ambiguous: %w", target, err), app.ExitAmbiguousOutcome)
		}
		return app.NewExitError(fmt.Errorf("container %s update outcome is ambiguous; do not retry until a fresh preflight completes", target), app.ExitAmbiguousOutcome)
	}
	verifyResult, err := runContainerOSAnsible(ctx, "verify-container-updates", host, vmid)
	if err != nil {
		return app.NewExitError(fmt.Errorf("container %s update completed but verification outcome is unknown; do not retry until a fresh verification completes: %w", target, err), app.ExitAmbiguousOutcome)
	}
	after := interpretContainerOSUpdate(verifyResult, hostName)
	verified := verifyResult.Success && after.EvidenceComplete && !after.DpkgIssues && len(after.PendingUpdates) == 0
	if err := writeContainerUpdateResult(cmdCtx, prov, profileName, target, true, verified, before, after); err != nil {
		if !verified {
			return app.NewExitError(fmt.Errorf("container %s update requires operator review after failed verification: %w", target, err), app.ExitAmbiguousOutcome)
		}
		return err
	}
	return nil
}

func writeContainerUpdateResult(cmdCtx *Context, prov domain.Provider, profileName, target string, submitted, verified bool, before, after containerUpdateState) error {
	result := output.NewOperationResult("container os-update", prov.Name(), profileName)
	result.Target = target
	result.Safety = safety.TierDisruptive.String()
	result.Submitted = submitted
	result.Waited = submitted
	result.Success = verified || !submitted
	changed := submitted && len(before.PendingUpdates) > len(after.PendingUpdates)
	result.Changed = &changed
	if result.Success {
		if submitted {
			result.Status = "verified"
		} else {
			result.Status = "no-updates"
		}
	} else {
		result.Status = "verification-failed"
		result.Error = &output.ResultError{Class: "verification_failed", Exit: app.ExitPartialFailure, Detail: "guest update postconditions were not satisfied"}
	}
	if after.RebootRequired {
		result.Warnings = append(result.Warnings, "guest reports reboot-required; Nodex did not reboot it")
	}
	if after.RootUsage != "" {
		if usage, parseErr := strconv.Atoi(strings.TrimSuffix(after.RootUsage, "%")); parseErr == nil && usage >= 90 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("guest root filesystem usage is %s", after.RootUsage))
		}
	}
	if err := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); err != nil {
		return err
	}
	if !result.Success {
		return app.NewExitError(fmt.Errorf("container update verification failed for %s", target), app.ExitPartialFailure)
	}
	return nil
}
