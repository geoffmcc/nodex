package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

const (
	vmGuestExecInitialPoll = 250 * time.Millisecond
	vmGuestExecMaxPoll     = 2 * time.Second
	vmGuestExecMaxWait     = 20 * time.Minute
)

func runVMPackageInstall(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 || len(args) > 33 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm install <node>/<vmid> <package> [package ...] (maximum 32 packages)"), app.ExitUsage)
	}
	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	packages := append([]string(nil), args[1:]...)
	if err := validateContainerPackageNames(packages); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if err := checkDisruptive(cmdCtx, fmt.Sprintf("install packages %v in QEMU VM %s/%d through its guest agent", packages, node, vmid)); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	statusInspector, ok := prov.(domain.GuestStatusInspector)
	if !ok {
		return app.NewExitError(fmt.Errorf("provider does not support authoritative VM status inspection"), app.ExitUnsupportedCap)
	}
	status, err := statusInspector.GuestStatus(ctx, node, "vm", vmid)
	if err != nil {
		return fmt.Errorf("inspect VM %s/%d before package installation: %w", node, vmid, err)
	}
	if status != "running" {
		return app.NewExitError(fmt.Errorf("VM %s/%d is %s; it must be running", node, vmid, status), app.ExitValidationError)
	}
	executor, ok := prov.(domain.VMGuestAgentExecutor)
	if !ok {
		return app.NewExitError(fmt.Errorf("provider does not support QEMU Guest Agent command execution"), app.ExitUnsupportedCap)
	}
	changed, err := installVMPackages(ctx, executor, node, vmid, packages)
	if err != nil {
		return err
	}
	result := output.NewOperationResult("vm install", prov.Name(), cmdCtx.Opts.Profile)
	result.Target = fmt.Sprintf("%s/%d", node, vmid)
	result.Safety = safety.TierDisruptive.String()
	result.Submitted, result.Waited, result.Synchronous, result.Success = true, true, true, true
	result.Verification, result.Status = "verified", "OK"
	result.Changed = boolPtrValue(changed)
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result)
}

func installVMPackages(ctx context.Context, executor domain.VMGuestAgentExecutor, node string, vmid int, packages []string) (bool, error) {
	if _, _, err := runVMGuestCommand(ctx, executor, node, vmid, []string{"/usr/bin/test", "-x", "/usr/bin/apt-get"}); err != nil {
		return false, fmt.Errorf("VM %s/%d package installation preflight failed: %w", node, vmid, err)
	}
	if _, _, err := runVMGuestCommand(ctx, executor, node, vmid, []string{"/usr/bin/apt-get", "-qq", "-o", "DPkg::Lock::Timeout=120", "update"}); err != nil {
		return false, packageInstallPartialError(node, vmid, "APT metadata refresh", err)
	}
	simulationArgs := []string{"/usr/bin/apt-get", "-s", "--no-remove", "-o", "DPkg::Lock::Timeout=120", "install", "--"}
	simulationArgs = append(simulationArgs, packages...)
	simulation, _, err := runVMGuestCommand(ctx, executor, node, vmid, simulationArgs)
	if err != nil {
		return false, packageInstallPartialError(node, vmid, "APT install simulation", err)
	}
	for _, line := range strings.Split(simulation, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Remv ") {
			return false, app.NewExitError(fmt.Errorf("VM %s/%d package installation was refused because APT would remove a package", node, vmid), app.ExitValidationError)
		}
	}
	changed := false
	for _, line := range strings.Split(simulation, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Inst ") {
			changed = true
			break
		}
	}
	installArgs := []string{"/usr/bin/env", "DEBIAN_FRONTEND=noninteractive", "/usr/bin/apt-get", "-qq", "-y", "--no-remove", "-o", "DPkg::Lock::Timeout=120", "install", "--"}
	installArgs = append(installArgs, packages...)
	_, _, err = runVMGuestCommand(ctx, executor, node, vmid, installArgs)
	if err != nil {
		return false, packageInstallPartialError(node, vmid, "APT install", err)
	}
	for _, packageName := range packages {
		verifyArgs := []string{"/usr/bin/dpkg-query", "-W", "-f=${Status}\n", "--", packageName}
		verified, _, verifyErr := runVMGuestCommand(ctx, executor, node, vmid, verifyArgs)
		if verifyErr != nil || !strings.Contains(verified, "install ok installed") {
			if verifyErr == nil {
				verifyErr = fmt.Errorf("package state did not confirm installed")
			}
			return false, packageInstallPartialError(node, vmid, "installed-package verification", verifyErr)
		}
	}
	return changed, nil
}

func runVMGuestCommand(ctx context.Context, executor domain.VMGuestAgentExecutor, node string, vmid int, argv []string) (string, string, error) {
	pid, err := executor.VMGuestExec(ctx, node, vmid, argv)
	if err != nil {
		return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent command submission outcome is unknown: %w", err), app.ExitAmbiguousOutcome)
	}
	if pid <= 0 {
		return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent returned no valid process ID; process outcome is unknown"), app.ExitAmbiguousOutcome)
	}
	deadline := time.NewTimer(vmGuestExecMaxWait)
	defer deadline.Stop()
	interval := vmGuestExecInitialPoll
	for {
		status, statusErr := executor.VMGuestExecStatus(ctx, node, vmid, pid)
		if statusErr != nil {
			return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent process %d outcome is unknown; do not resubmit automatically: %w", pid, statusErr), app.ExitAmbiguousOutcome)
		}
		if status == nil {
			return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent returned no status for process %d; outcome is unknown", pid), app.ExitAmbiguousOutcome)
		}
		if status.Exited {
			if status.ExitCode == nil {
				return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent process %d exited without an exit code; outcome is unknown", pid), app.ExitAmbiguousOutcome)
			}
			if *status.ExitCode != 0 {
				return status.Stdout, status.Stderr, fmt.Errorf("guest process exited with code %d", *status.ExitCode)
			}
			return status.Stdout, status.Stderr, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent process %d may still be running after local cancellation; outcome is unknown", pid), app.ExitAmbiguousOutcome)
		case <-deadline.C:
			timer.Stop()
			return "", "", app.NewExitError(fmt.Errorf("QEMU Guest Agent process %d may still be running after the 20 minute wait limit; outcome is unknown", pid), app.ExitAmbiguousOutcome)
		case <-timer.C:
		}
		if interval < vmGuestExecMaxPoll/2 {
			interval *= 2
		} else {
			interval = vmGuestExecMaxPoll
		}
	}
}

func packageInstallPartialError(node string, vmid int, phase string, err error) error {
	var exitCoder *app.ExitCoder
	if errors.As(err, &exitCoder) && exitCoder.ExitCode == app.ExitAmbiguousOutcome {
		return app.NewExitError(fmt.Errorf("VM %s/%d package installation outcome is unknown during %s; do not retry automatically: %w", node, vmid, phase, err), app.ExitAmbiguousOutcome)
	}
	return app.NewExitError(fmt.Errorf("VM %s/%d package installation partially completed or could not be verified during %s: %w", node, vmid, phase, err), app.ExitPartialFailure)
}
