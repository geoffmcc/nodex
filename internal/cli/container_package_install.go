package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/safety"
)

var guestPackageNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*(?::[a-z0-9][a-z0-9-]*)?$`)

var runContainerPackageInstallAnsible = func(ctx context.Context, host ansible.HostSpec, vmid int, packages []string) (*ansible.RunResult, error) {
	det, err := ansible.Detect(ctx)
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("container package install requires Ansible: %w", err), app.ExitIncompatibility)
	}
	return (&ansible.Runner{Exe: det.Path}).Run(ctx, ansible.RunRequest{
		Operation: "install-container-packages", Hosts: []ansible.HostSpec{host},
		ContainerVMID: vmid, ContainerPackages: packages,
	})
}

func runContainerPackageInstall(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 || len(args) > 33 {
		return app.NewExitError(fmt.Errorf("usage: nodex container install <node>/<vmid> <package> [package ...] (maximum 32 packages)"), app.ExitUsage)
	}
	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	packages := append([]string(nil), args[1:]...)
	if err := validateContainerPackageNames(packages); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	profileName, err := resolveProfileName(cmdCtx)
	if err != nil {
		return err
	}
	cfg := cmdCtx.AgentConfig
	if cfg == nil {
		cfg, err = config.Read()
		if err != nil {
			return err
		}
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
	inspector, err := requireContainerInspector(prov)
	if err != nil {
		return err
	}
	containers, err := inspector.Containers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	target := fmt.Sprintf("%s/%d", node, vmid)
	container, ok := findContainer(containers, target)
	if !ok {
		return app.NotFoundError("container %q", target)
	}
	if container.Status != "running" {
		return app.NewExitError(fmt.Errorf("container %s is %s; it must be running", target, container.Status), app.ExitValidationError)
	}
	if err := checkDisruptive(cmdCtx, fmt.Sprintf("install packages %v in LXC guest %s", packages, target)); err != nil {
		return err
	}
	host := ansible.HostSpec{
		Name: hostName, Address: inventoryHost.Address, Port: inventoryHost.SSHPort,
		User: inventoryHost.SSHUser, KeyFile: inventoryHost.SSHKeyFile,
		KnownHostsFile: inventoryHost.KnownHostsFile,
	}
	result, err := runContainerPackageInstallAnsible(ctx, host, vmid, packages)
	if err != nil {
		return app.NewExitError(fmt.Errorf("container %s package installation outcome is unknown: %w", target, err), app.ExitAmbiguousOutcome)
	}
	if result == nil || !result.Success || !result.EvidenceComplete {
		return app.NewExitError(fmt.Errorf("container %s package installation failed or could not be verified: %s", target, containerPackageInstallFailure(result, hostName)), app.ExitPartialFailure)
	}
	operation := output.NewOperationResult("container install", prov.Name(), profileName)
	operation.Target = target
	operation.Safety = safety.TierDisruptive.String()
	operation.Submitted = true
	operation.Waited = true
	operation.Success = true
	operation.Verification = "verified"
	operation.Changed = boolPtrValue(resultHostChanged(result, hostName))
	operation.Status = "OK"
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, operation)
}

func containerPackageInstallFailure(result *ansible.RunResult, hostName string) string {
	if result == nil {
		return "Ansible returned no result"
	}
	var details []string
	if result.ParseError != "" {
		details = append(details, result.ParseError)
	}
	if missing := result.MissingEvidence[hostName]; len(missing) > 0 {
		details = append(details, "missing evidence: "+strings.Join(missing, ", "))
	}
	for _, host := range result.Hosts {
		if host.Host == hostName && (host.Failed || host.Failures > 0 || host.Unreachable > 0) {
			details = append(details, fmt.Sprintf("host failures=%d unreachable=%d", host.Failures, host.Unreachable))
		}
	}
	for _, task := range result.TaskOutcomes[hostName] {
		if !task.Failed && !task.Unreachable && (task.RC == nil || *task.RC == 0) {
			continue
		}
		var taskDetails []string
		if message := strings.TrimSpace(task.Message); message != "" {
			taskDetails = append(taskDetails, message)
		}
		if stderr := strings.TrimSpace(strings.Join(task.StderrLines, " ")); stderr != "" {
			taskDetails = append(taskDetails, "stderr: "+stderr)
		}
		if stdout := strings.TrimSpace(strings.Join(task.StdoutLines, " ")); stdout != "" {
			taskDetails = append(taskDetails, "stdout: "+stdout)
		}
		detail := strings.Join(taskDetails, "; ")
		if detail == "" {
			detail = "task failed"
		}
		if task.RC != nil && *task.RC != 0 {
			detail = fmt.Sprintf("rc=%d: %s", *task.RC, detail)
		}
		if len(detail) > 1000 {
			detail = detail[:1000] + "…"
		}
		details = append(details, task.Task+": "+detail)
	}
	if len(details) == 0 {
		return "Ansible run did not satisfy its evidence contract"
	}
	return redact.String(output.SanitizeTerminal(strings.Join(details, "; ")))
}

func validateContainerPackageNames(packages []string) error {
	if len(packages) == 0 || len(packages) > 32 {
		return fmt.Errorf("provide between 1 and 32 package names")
	}
	seen := map[string]bool{}
	for _, name := range packages {
		if len(name) > 128 || !guestPackageNameRe.MatchString(name) {
			return fmt.Errorf("invalid package name %q; use a Debian package name such as curl or python3-venv", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate package name %q", name)
		}
		seen[name] = true
	}
	return nil
}

func resultHostChanged(result *ansible.RunResult, host string) bool {
	if result == nil {
		return false
	}
	for _, h := range result.Hosts {
		if h.Host == host {
			return h.Changed > 0
		}
	}
	return false
}

func boolPtrValue(value bool) *bool { return &value }
