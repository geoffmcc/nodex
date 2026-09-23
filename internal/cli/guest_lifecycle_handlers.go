package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/task"
)

// taskStatusAdapter adapts a domain.TaskInspector to the task.TaskStatusClient interface.
type taskStatusAdapter struct {
	ti domain.TaskInspector
}

func (a *taskStatusAdapter) GetTask(ctx context.Context, node, upid string) (*task.TaskStatus, error) {
	t, err := a.ti.Task(ctx, node, upid)
	if err != nil {
		return nil, err
	}
	state := task.StateRunning
	if t.State == "stopped" {
		state = task.StateStopped
	}
	return &task.TaskStatus{
		UPID:   t.UPID,
		State:  state,
		Status: t.Status,
	}, nil
}

// requireLifecycle checks if the provider supports LifecycleProvider.
func requireLifecycle(prov domain.Provider) (domain.LifecycleProvider, error) {
	p, ok := prov.(domain.LifecycleProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: lifecycle commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// parseNodeVMID parses a "<node>/<vmid>" argument.
func parseNodeVMID(arg string) (node string, vmid int, err error) {
	parts := strings.SplitN(arg, "/", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid target %q: expected <node>/<vmid>", arg)
	}
	node = parts[0]
	if node == "" {
		return "", 0, fmt.Errorf("node name is required")
	}
	vmid, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid VMID %q: %w", parts[1], err)
	}
	if vmid <= 0 {
		return "", 0, fmt.Errorf("VMID must be positive")
	}
	return node, vmid, nil
}

// lifecycleDesiredState returns the guest status that indicates a lifecycle
// operation is already satisfied (a no-op). It returns "" when the operation
// has no meaningful desired state (e.g. reset/reboot always take effect).
func lifecycleDesiredState(resourceType, operation string) string {
	switch resourceType {
	case "vm":
		switch operation {
		case "start", "resume", "unpause":
			return "running"
		case "stop", "shutdown":
			return "stopped"
		case "suspend", "pause":
			return "paused"
		}
	case "container":
		switch operation {
		case "start", "resume":
			return "running"
		case "stop", "shutdown":
			return "stopped"
		case "suspend":
			return "paused"
		}
	}
	return ""
}

// currentGuestStatus returns the reported status of a guest when the provider
// exposes guest state and the guest is listed. ok=false means the state could
// not be determined and callers should proceed with a normal submit.
func currentGuestStatus(ctx context.Context, prov domain.Provider, resourceType, node string, vmid int) (string, bool) {
	id := fmt.Sprintf("%s/%d", node, vmid)
	if resourceType == "vm" {
		vi, ok := prov.(domain.VMInspector)
		if !ok {
			return "", false
		}
		vms, err := vi.VMs(ctx)
		if err != nil {
			return "", false
		}
		vm, found := findVM(vms, id)
		if !found {
			return "", false
		}
		return vm.Status, true
	}
	ci, ok := prov.(domain.ContainerInspector)
	if !ok {
		return "", false
	}
	cts, err := ci.Containers(ctx)
	if err != nil {
		return "", false
	}
	ct, found := findContainer(cts, id)
	if !found {
		return "", false
	}
	return ct.Status, true
}

// isBenignLifecycleStatus reports whether a provider status/error string
// indicates the lifecycle operation was already satisfied on the guest side
// (e.g. PVE rejecting a start with "VM 9999 already running"). Only
// reach-a-state operations with a defined desired state qualify; reset/reboot
// never do because they take effect on a running guest.
func isBenignLifecycleStatus(resourceType, operation, status string) bool {
	switch lifecycleDesiredState(resourceType, operation) {
	case "":
		return false
	case "running":
		lower := strings.ToLower(status)
		return strings.Contains(lower, "already running") || strings.Contains(lower, "already started")
	case "stopped":
		lower := strings.ToLower(status)
		return strings.Contains(lower, "not running") || strings.Contains(lower, "already stopped") || strings.Contains(lower, "already shutdown")
	case "paused":
		return strings.Contains(strings.ToLower(status), "already paused")
	}
	return false
}

// runLifecycle executes a VM lifecycle operation with safety checks and optional task polling.
func runLifecycle(ctx context.Context, cmdCtx *Context, args []string, operation, resourceType string, tier safety.Tier) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex %s %s <node>/<vmid>", resourceType, operation), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	lc, err := requireLifecycle(prov)
	if err != nil {
		return err
	}
	if resourceType == "vm" && operation == "start" {
		if vi, ok := prov.(domain.VMInspector); ok {
			if vms, listErr := vi.VMs(ctx); listErr == nil {
				if vm, found := findVM(vms, fmt.Sprintf("%s/%d", node, vmid)); found && vm.Template {
					return app.NewExitError(fmt.Errorf("cannot start VM %s/%d: it is a template", node, vmid), app.ExitValidationError)
				}
			}
		}
	}

	// Safety check.
	desc := fmt.Sprintf("%s %s/%d", resourceType, node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                tier,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
	}

	// Idempotent pre-check: if the guest is already in the desired state,
	// report success with a note and skip submission entirely.
	if desired := lifecycleDesiredState(resourceType, operation); desired != "" {
		if state, ok := currentGuestStatus(ctx, prov, resourceType, node, vmid); ok && strings.EqualFold(state, desired) {
			profileName, _ := resolveProfileName(cmdCtx)
			opResult := output.NewOperationResult(resourceType+" "+operation, prov.Name(), profileName)
			opResult.Target = fmt.Sprintf("%s/%d", node, vmid)
			opResult.Safety = tier.String()
			opResult.Success = true
			opResult.Status = fmt.Sprintf("already %s", state)
			return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
		}
	}

	// Execute operation.
	upid, err := executeLifecycleOp(ctx, lc, resourceType, operation, node, vmid)
	if err != nil {
		// A race between the pre-check and submit can still surface a
		// benign "already in state" rejection from the provider.
		if isBenignLifecycleStatus(resourceType, operation, err.Error()) {
			profileName, _ := resolveProfileName(cmdCtx)
			opResult := output.NewOperationResult(resourceType+" "+operation, prov.Name(), profileName)
			opResult.Target = fmt.Sprintf("%s/%d", node, vmid)
			opResult.Safety = tier.String()
			opResult.Success = true
			opResult.Status = "already in desired state"
			return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
		}
		return fmt.Errorf("%s %s %s/%d: %w", resourceType, operation, node, vmid, err)
	}

	profileName, _ := resolveProfileName(cmdCtx)
	opResult := output.NewOperationResult(resourceType+" "+operation, prov.Name(), profileName)
	opResult.Target = fmt.Sprintf("%s/%d", node, vmid)
	opResult.Safety = tier.String()
	opResult.UPID = upid
	opResult.Submitted = true
	opResult.Success = true

	// If not waiting, write result and exit.
	if !cmdCtx.Opts.Wait {
		return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
	}
	if upid == "" {
		opResult.Success = false
		opResult.Status = "ambiguous"
		opResult.Error = &output.ResultError{
			Class:  "ambiguous_outcome",
			Exit:   app.ExitAmbiguousOutcome,
			Detail: "provider returned no task ID; completion cannot be verified",
		}
		if err := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult); err != nil {
			return err
		}
		return app.NewExitError(errors.New("provider returned no task ID; completion cannot be verified"), app.ExitAmbiguousOutcome)
	}

	// Wait for task to complete.
	fmt.Fprintf(cmdCtx.ErrW, "Waiting for task %s...\n", upid)
	ti, ok := prov.(domain.TaskInspector)
	if !ok {
		return app.NewExitError(
			fmt.Errorf("%w: task polling not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	adapter := &taskStatusAdapter{ti: ti}
	poller := task.NewPoller(adapter)
	tr := poller.Wait(ctx, node, upid)

	opResult.Waited = true
	if tr.Error != nil {
		opResult.Success = false
		exitCode := classifyTaskError(tr.Error, upid)
		opResult.Error = &output.ResultError{
			Class:  exitClassFromCode(exitCode),
			Exit:   exitCode,
			Detail: tr.Error.Error(),
		}
		_ = output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
		return app.NewExitError(
			&app.ProviderError{UPID: upid, Detail: tr.Error.Error(), Err: tr.Error},
			exitCode,
		)
	}
	if !tr.OK {
		// The task may have been rejected because the guest is already in the
		// desired state (a race between the pre-check and the task submission).
		if isBenignLifecycleStatus(resourceType, operation, tr.Status) {
			opResult.Success = true
			opResult.Status = tr.Status
			return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
		}
		opResult.Success = false
		opResult.Error = &output.ResultError{
			Class:  "task_failure",
			Exit:   app.ExitTaskFailure,
			Detail: fmt.Sprintf("task failed with status %q", tr.Status),
		}
		_ = output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
		return app.NewExitError(
			fmt.Errorf("task %s failed with status %q", upid, tr.Status),
			app.ExitTaskFailure,
		)
	}
	opResult.Status = "OK"
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
}

// executeLifecycleOp calls the appropriate lifecycle method.
func executeLifecycleOp(ctx context.Context, lc domain.LifecycleProvider, resourceType, operation, node string, vmid int) (string, error) {
	if resourceType == "vm" {
		switch operation {
		case "start":
			return lc.VMStart(ctx, node, vmid)
		case "stop":
			return lc.VMStop(ctx, node, vmid)
		case "shutdown":
			return lc.VMShutdown(ctx, node, vmid)
		case "reset":
			return lc.VMReset(ctx, node, vmid)
		case "reboot":
			return lc.VMReboot(ctx, node, vmid)
		case "suspend":
			return lc.VMSuspend(ctx, node, vmid)
		case "resume":
			return lc.VMResume(ctx, node, vmid)
		case "pause":
			return lc.VMPause(ctx, node, vmid)
		case "unpause":
			return lc.VMUnpause(ctx, node, vmid)
		}
	} else if resourceType == "container" {
		switch operation {
		case "start":
			return lc.CTStart(ctx, node, vmid)
		case "stop":
			return lc.CTStop(ctx, node, vmid)
		case "shutdown":
			return lc.CTShutdown(ctx, node, vmid)
		case "reboot":
			return lc.CTReboot(ctx, node, vmid)
		case "suspend":
			return lc.CTSuspend(ctx, node, vmid)
		case "resume":
			return lc.CTResume(ctx, node, vmid)
		}
	}
	return "", fmt.Errorf("unknown operation %s for resource type %s", operation, resourceType)
}

// === VM Lifecycle Handlers ===

func runVMStart(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "start", "vm", safety.TierReversible)
}

func runVMStop(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "stop", "vm", safety.TierReversible)
}

func runVMShutdown(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "shutdown", "vm", safety.TierReversible)
}

func runVMReset(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "reset", "vm", safety.TierDisruptive)
}

func runVMReboot(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "reboot", "vm", safety.TierDisruptive)
}

func runVMSuspend(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "suspend", "vm", safety.TierReversible)
}

func runVMResume(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "resume", "vm", safety.TierReversible)
}

func runVMPause(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "pause", "vm", safety.TierReversible)
}

func runVMUnpause(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "unpause", "vm", safety.TierReversible)
}

// === Container Lifecycle Handlers ===

func runCTStart(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "start", "container", safety.TierReversible)
}

func runCTStop(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "stop", "container", safety.TierReversible)
}

func runCTShutdown(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "shutdown", "container", safety.TierReversible)
}

func runCTReboot(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "reboot", "container", safety.TierDisruptive)
}

func runCTSuspend(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "suspend", "container", safety.TierReversible)
}

func runCTResume(ctx context.Context, cmdCtx *Context, args []string) error {
	return runLifecycle(ctx, cmdCtx, args, "resume", "container", safety.TierReversible)
}
