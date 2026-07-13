package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/task"
)

// --- Config Update Handlers ---

// parseKeyValueArgs parses key=value pairs from a slice of strings.
// Returns a map of keys to values, and an error if any arg is malformed.
func parseKeyValueArgs(args []string) (map[string]string, error) {
	params := make(map[string]string)
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter %q: expected key=value format", arg)
		}
		key := parts[0]
		if key == "" {
			return nil, fmt.Errorf("invalid parameter %q: empty key", arg)
		}
		params[key] = parts[1]
	}
	return params, nil
}

// requireConfig checks if the provider supports ConfigProvider.
func requireConfig(prov domain.Provider) (domain.ConfigProvider, error) {
	p, ok := prov.(domain.ConfigProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: config commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireSnapshotMutation checks if the provider supports SnapshotMutationProvider.
func requireSnapshotMutation(prov domain.Provider) (domain.SnapshotMutationProvider, error) {
	p, ok := prov.(domain.SnapshotMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: snapshot mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireDelete checks if the provider supports DeleteProvider.
func requireDelete(prov domain.Provider) (domain.DeleteProvider, error) {
	p, ok := prov.(domain.DeleteProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: delete commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireTemplate checks if the provider supports TemplateProvider.
func requireTemplate(prov domain.Provider) (domain.TemplateProvider, error) {
	p, ok := prov.(domain.TemplateProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: template commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// requireCloudInit checks if the provider supports CloudInitProvider.
func requireCloudInit(prov domain.Provider) (domain.CloudInitProvider, error) {
	p, ok := prov.(domain.CloudInitProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: cloud-init commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// runMutationWithPolling executes a mutation, prints the UPID, and optionally polls the task.
func runMutationWithPolling(ctx context.Context, cmdCtx *Context, prov domain.Provider, node, upid string) error {
	if !cmdCtx.Opts.Wait {
		fmt.Fprintf(cmdCtx.Writer, "%s\n", upid)
		return nil
	}

	fmt.Fprintf(cmdCtx.ErrW, "Waiting for task %s...\n", upid)
	adapter := &taskStatusAdapter{prov: prov}
	poller := task.NewPoller(adapter)
	tr := poller.Wait(ctx, node, upid)
	if tr.Error != nil {
		fmt.Fprintf(cmdCtx.ErrW, "Task %s: %v\n", upid, tr.Error)
	}
	if tr.OK {
		fmt.Fprintf(cmdCtx.Writer, "%s (completed OK)\n", upid)
	} else {
		fmt.Fprintf(cmdCtx.Writer, "%s (status: %s)\n", upid, tr.State)
	}
	return nil
}

// --- VM Config Update ---

func runVMUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm update <node>/<vmid> <key=value>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	params, err := parseKeyValueArgs(args[1:])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if len(params) == 0 {
		return app.NewExitError(fmt.Errorf("at least one key=value parameter is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cp, err := requireConfig(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d", node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return nil
	}

	upid, err := cp.VMConfigUpdate(ctx, node, vmid, params)
	if err != nil {
		return fmt.Errorf("update VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Config Update ---

func runCTUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container update <node>/<vmid> <key=value>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	params, err := parseKeyValueArgs(args[1:])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if len(params) == 0 {
		return app.NewExitError(fmt.Errorf("at least one key=value parameter is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cp, err := requireConfig(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d", node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return nil
	}

	upid, err := cp.CTConfigUpdate(ctx, node, vmid, params)
	if err != nil {
		return fmt.Errorf("update container %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Snapshot Create ---

func runVMSnapshotCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot create <node>/<vmid> <name> [description]"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]
	description := ""
	if len(args) > 2 {
		description = args[2]
	}
	if len(args) > 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot create <node>/<vmid> <name> [description]"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d snapshot %q", node, vmid, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return nil
	}

	upid, err := sp.VMSnapshotCreate(ctx, node, vmid, name, description)
	if err != nil {
		return fmt.Errorf("create VM snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Snapshot Delete (Tier 3: destructive) ---

func runVMSnapshotDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot delete <node>/<vmid> <name>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/%d", node, vmid)
	desc := fmt.Sprintf("VM %s snapshot %q", targetID, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDestructive,
		ResourceDescription: desc,
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   name,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		if result.TypeConfirmRequired && !result.DoubleConfirmRequired && cmdCtx.Opts.Yes && cmdCtx.Opts.Force {
			// Already passed --yes --force, now need type-in confirmation.
			fmt.Fprintf(cmdCtx.ErrW, "%s", result.Message)
			reader := bufio.NewReader(os.Stdin)
			typed, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			typed = strings.TrimSpace(typed)
			if typed != name {
				return app.NewExitError(
					fmt.Errorf("typed %q does not match snapshot name %q — operation cancelled", typed, name),
					app.ExitUsage,
				)
			}
			// Confirmed via type-in.
		} else {
			fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
			return nil
		}
	}

	upid, err := sp.VMSnapshotDelete(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("delete VM snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Snapshot Rollback (Tier 2: disruptive) ---

func runVMSnapshotRollback(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm snapshot rollback <node>/<vmid> <name>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d rollback to snapshot %q", node, vmid, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
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
		return nil
	}

	upid, err := sp.VMSnapshotRollback(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("rollback VM snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Snapshot Create ---

func runCTSnapshotCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot create <node>/<vmid> <name> [description]"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]
	description := ""
	if len(args) > 2 {
		description = args[2]
	}
	if len(args) > 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot create <node>/<vmid> <name> [description]"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d snapshot %q", node, vmid, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return nil
	}

	upid, err := sp.CTSnapshotCreate(ctx, node, vmid, name, description)
	if err != nil {
		return fmt.Errorf("create container snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Snapshot Delete (Tier 3: destructive) ---

func runCTSnapshotDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot delete <node>/<vmid> <name>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d snapshot %q", node, vmid, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDestructive,
		ResourceDescription: desc,
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   name,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		if result.TypeConfirmRequired && !result.DoubleConfirmRequired && cmdCtx.Opts.Yes && cmdCtx.Opts.Force {
			fmt.Fprintf(cmdCtx.ErrW, "%s", result.Message)
			reader := bufio.NewReader(os.Stdin)
			typed, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			typed = strings.TrimSpace(typed)
			if typed != name {
				return app.NewExitError(
					fmt.Errorf("typed %q does not match snapshot name %q — operation cancelled", typed, name),
					app.ExitUsage,
				)
			}
		} else {
			fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
			return nil
		}
	}

	upid, err := sp.CTSnapshotDelete(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("delete container snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Snapshot Rollback (Tier 2: disruptive) ---

func runCTSnapshotRollback(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container snapshot rollback <node>/<vmid> <name>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	name := args[1]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireSnapshotMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d rollback to snapshot %q", node, vmid, name)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
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
		return nil
	}

	upid, err := sp.CTSnapshotRollback(ctx, node, vmid, name)
	if err != nil {
		return fmt.Errorf("rollback container snapshot %s/%d/%s: %w", node, vmid, name, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Delete (Tier 3: destructive) ---

func runVMDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm delete <node>/<vmid>"), app.ExitUsage)
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

	dp, err := requireDelete(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/%d", node, vmid)
	desc := fmt.Sprintf("VM %s", targetID)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDestructive,
		ResourceDescription: desc,
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   targetID,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		if result.TypeConfirmRequired && !result.DoubleConfirmRequired && cmdCtx.Opts.Yes && cmdCtx.Opts.Force {
			// Already passed --yes --force, now need type-in confirmation.
			fmt.Fprintf(cmdCtx.ErrW, "%s", result.Message)
			reader := bufio.NewReader(os.Stdin)
			typed, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			typed = strings.TrimSpace(typed)
			if typed != targetID {
				return app.NewExitError(
					fmt.Errorf("typed %q does not match target %q — operation cancelled", typed, targetID),
					app.ExitUsage,
				)
			}
			// Confirmed via type-in.
		} else {
			fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
			return nil
		}
	}

	upid, err := dp.VMDelete(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("delete VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Delete (Tier 3: destructive) ---

func runCTDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container delete <node>/<vmid>"), app.ExitUsage)
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

	dp, err := requireDelete(prov)
	if err != nil {
		return err
	}

	targetID := fmt.Sprintf("%s/%d", node, vmid)
	desc := fmt.Sprintf("container %s", targetID)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDestructive,
		ResourceDescription: desc,
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   targetID,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		if result.TypeConfirmRequired && !result.DoubleConfirmRequired && cmdCtx.Opts.Yes && cmdCtx.Opts.Force {
			fmt.Fprintf(cmdCtx.ErrW, "%s", result.Message)
			reader := bufio.NewReader(os.Stdin)
			typed, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("read confirmation: %w", err)
			}
			typed = strings.TrimSpace(typed)
			if typed != targetID {
				return app.NewExitError(
					fmt.Errorf("typed %q does not match target %q — operation cancelled", typed, targetID),
					app.ExitUsage,
				)
			}
		} else {
			fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
			return nil
		}
	}

	upid, err := dp.CTDelete(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("delete container %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Cloud-Init (Tier 1: reversible) ---

func runVMCloudInit(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm cloud-init <node>/<vmid>"), app.ExitUsage)
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

	cp, err := requireCloudInit(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d", node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return nil
	}

	upid, err := cp.VMCloudInit(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("cloud-init VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- VM Template (Tier 2: disruptive) ---

func runVMTemplate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm template <node>/<vmid>"), app.ExitUsage)
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

	tp, err := requireTemplate(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d -> template", node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
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
		return nil
	}

	upid, err := tp.VMTemplate(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("template VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Container Template (Tier 2: disruptive) ---

func runCTTemplate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex container template <node>/<vmid>"), app.ExitUsage)
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

	tp, err := requireTemplate(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d -> template", node, vmid)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
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
		return nil
	}

	upid, err := tp.CTTemplate(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("template container %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid)
}

// --- Snapshot Dispatch Handlers ---

// runVMSnapshotDispatch routes to the appropriate VM snapshot sub-command.
func runVMSnapshotDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex vm snapshot <create|delete|rollback> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  create   <node>/<vmid> <name> [description]")
		fmt.Fprintln(cmdCtx.Writer, "  delete   <node>/<vmid> <name>")
		fmt.Fprintln(cmdCtx.Writer, "  rollback <node>/<vmid> <name>")
		return nil
	}
	switch args[0] {
	case "create":
		return runVMSnapshotCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runVMSnapshotDelete(ctx, cmdCtx, args[1:])
	case "rollback":
		return runVMSnapshotRollback(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown vm snapshot subcommand: %s (use create, delete, or rollback)", args[0]),
			app.ExitUsage,
		)
	}
}

// runCTSnapshotDispatch routes to the appropriate container snapshot sub-command.
func runCTSnapshotDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex container snapshot <create|delete|rollback> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  create   <node>/<vmid> <name> [description]")
		fmt.Fprintln(cmdCtx.Writer, "  delete   <node>/<vmid> <name>")
		fmt.Fprintln(cmdCtx.Writer, "  rollback <node>/<vmid> <name>")
		return nil
	}
	switch args[0] {
	case "create":
		return runCTSnapshotCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runCTSnapshotDelete(ctx, cmdCtx, args[1:])
	case "rollback":
		return runCTSnapshotRollback(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown container snapshot subcommand: %s (use create, delete, or rollback)", args[0]),
			app.ExitUsage,
		)
	}
}
