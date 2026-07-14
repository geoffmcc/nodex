package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/task"
)

// taskStatusAdapter adapts a domain.Provider to the task.TaskStatusClient interface.
type taskStatusAdapter struct {
	prov domain.Provider
}

func (a *taskStatusAdapter) GetTask(ctx context.Context, node, upid string) (*task.TaskStatus, error) {
	t, err := a.prov.Task(ctx, node, upid)
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

	// Execute operation.
	upid, err := executeLifecycleOp(ctx, lc, resourceType, operation, node, vmid)
	if err != nil {
		return fmt.Errorf("%s %s %s/%d: %w", resourceType, operation, node, vmid, err)
	}

	// If not waiting, just print UPID and exit.
	if !cmdCtx.Opts.Wait {
		fmt.Fprintf(cmdCtx.Writer, "%s\n", upid)
		return nil
	}

	// Wait for task to complete.
	fmt.Fprintf(cmdCtx.ErrW, "Waiting for task %s...\n", upid)
	adapter := &taskStatusAdapter{prov: prov}
	poller := task.NewPoller(adapter)
	tr := poller.Wait(ctx, node, upid)
	if tr.Error != nil {
		return app.NewExitError(
			fmt.Errorf("task %s failed: %w", upid, tr.Error),
			app.ExitProvider,
		)
	}
	if tr.OK {
		fmt.Fprintf(cmdCtx.Writer, "%s (completed OK)\n", upid)
		return nil
	}
	return app.NewExitError(
		fmt.Errorf("task %s failed with status %q", upid, tr.State),
		app.ExitProvider,
	)
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
