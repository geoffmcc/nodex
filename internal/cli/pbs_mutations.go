package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
	"github.com/geoffmcc/nodex/internal/task"
)

// Guarded PBS mutations: verify run, sync run, prune run, garbage-collection
// run. Every handler follows the same sequence: parse → connect → require
// capability → resolve target job/datastore → safety confirmation →
// conflicting-task preflight → execute → OperationResult (+ optional --wait
// task polling). Non-interactive mode fails closed on every confirmation.

// pbsTaskStatusAdapter adapts a domain.PBSTaskInspector to the
// task.TaskStatusClient interface. The node argument is unused: PBS task
// paths address the local node.
type pbsTaskStatusAdapter struct {
	ti domain.PBSTaskInspector
}

func (a *pbsTaskStatusAdapter) GetTask(ctx context.Context, _, upid string) (*task.TaskStatus, error) {
	s, err := a.ti.PBSTaskStatus(ctx, upid)
	if err != nil {
		return nil, err
	}
	state := task.StateRunning
	status := s.Status
	if s.Status == "stopped" {
		state = task.StateStopped
		status = s.ExitStatus
	}
	return &task.TaskStatus{UPID: s.UPID, State: state, Status: status}, nil
}

// pbsConflictWorkerTypes are the running worker types that block a new
// CLI-triggered maintenance operation on the same datastore. Conservative by
// design: PBS enforces its own locking, but Nodex refuses up front rather
// than starting work that will contend with backups or other maintenance.
var pbsConflictWorkerTypes = map[string]bool{
	"backup":             true,
	"garbage_collection": true,
	"prune":              true,
	"prunejob":           true,
	"sync":               true,
	"syncjob":            true,
	"verify":             true,
	"verificationjob":    true,
	"verify_group":       true,
	"verify_snapshot":    true,
}

// pbsCheckConflictingTasks refuses (ExitConflict) when a running PBS task of
// a conflicting worker type targets the given datastore. Store matching is
// against the task's worker ID (exact or "<store>:..." prefixed).
func pbsCheckConflictingTasks(ctx context.Context, prov domain.Provider, store string) error {
	ti, ok := prov.(domain.PBSTaskInspector)
	if !ok {
		// A provider that implements runners but not task inspection cannot
		// be preflighted; fail closed rather than guessing.
		return app.NewExitError(
			fmt.Errorf("%w: provider %q cannot list tasks for conflict preflight", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	running, err := ti.PBSTasks(ctx, domain.PBSTaskFilter{Running: true})
	if err != nil {
		return fmt.Errorf("conflict preflight: list running tasks: %w", err)
	}
	for _, t := range running {
		if !pbsConflictWorkerTypes[t.WorkerType] {
			continue
		}
		if t.WorkerID == store || strings.HasPrefix(t.WorkerID, store+":") {
			return app.NewExitError(
				fmt.Errorf("conflicting PBS task is running on datastore %q: %s (%s); wait for it to finish",
					store, t.WorkerType, t.UPID),
				app.ExitConflict,
			)
		}
	}
	return nil
}

// checkReversible verifies Tier 1 authorization (--yes).
func checkReversible(cmdCtx *Context, desc string) error {
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierReversible,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if !result.ConfirmationRequired {
		return nil
	}
	if cmdCtx.Opts.NonInteractive {
		return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
	}
	if result.Warning != "" {
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
	}
	fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
	return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
}

// pbsWriteMutationResult writes the OperationResult envelope and, with
// --wait, polls the task to completion first.
func pbsWriteMutationResult(ctx context.Context, cmdCtx *Context, prov domain.Provider, operation, target string, tier safety.Tier, upid string, warnings []string) error {
	profileName, _ := resolveProfileName(cmdCtx)
	opResult := output.NewOperationResult(operation, prov.Name(), profileName)
	opResult.Target = target
	opResult.Safety = tier.String()
	opResult.UPID = upid
	opResult.Submitted = true
	opResult.Success = true
	opResult.Warnings = warnings

	if !cmdCtx.Opts.Wait {
		return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
	}
	if upid == "" {
		opResult.Warnings = append(opResult.Warnings, "provider returned no task ID; cannot wait")
		return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, opResult)
	}

	ti, ok := prov.(domain.PBSTaskInspector)
	if !ok {
		return app.NewExitError(
			fmt.Errorf("%w: task polling not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	fmt.Fprintf(cmdCtx.ErrW, "Waiting for task %s...\n", upid)
	poller := task.NewPoller(&pbsTaskStatusAdapter{ti: ti})
	tr := poller.Wait(ctx, "localhost", upid)

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

// pbsFindVerifyJob returns the configured verify job with the given ID.
func pbsFindVerifyJob(ctx context.Context, prov domain.Provider, id string) (*domain.PBSVerifyJob, error) {
	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return nil, err
	}
	items, err := jobs.PBSVerifyJobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list verify jobs: %w", err)
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, app.NewExitError(
		fmt.Errorf("verify job %q not found", id),
		app.ExitNotFound,
	)
}

// === pbs verify run ===

func runPBSVerifyRun(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs verify run <job-id> | --datastore <store>")
	jobID, store := "", ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--datastore":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return app.NewExitError(usage, app.ExitUsage)
			}
			store = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--datastore="):
			store = strings.TrimPrefix(args[i], "--datastore=")
			if store == "" {
				return app.NewExitError(usage, app.ExitUsage)
			}
		case strings.HasPrefix(args[i], "--"):
			return app.NewExitError(usage, app.ExitUsage)
		default:
			if jobID != "" {
				return app.NewExitError(usage, app.ExitUsage)
			}
			jobID = args[i]
		}
	}
	if (jobID == "") == (store == "") {
		// Exactly one of job ID or --datastore is required.
		return app.NewExitError(usage, app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	runner, err := requirePBSVerifyRun(prov)
	if err != nil {
		return err
	}

	target, conflictStore := store, store
	if jobID != "" {
		job, err := pbsFindVerifyJob(ctx, prov, jobID)
		if err != nil {
			return err
		}
		target, conflictStore = jobID, job.Store
	}

	if err := checkReversible(cmdCtx, fmt.Sprintf("pbs verify run %s", target)); err != nil {
		return err
	}
	if err := pbsCheckConflictingTasks(ctx, prov, conflictStore); err != nil {
		return err
	}

	var upid string
	if jobID != "" {
		upid, err = runner.PBSRunVerifyJob(ctx, jobID)
	} else {
		upid, err = runner.PBSVerifyDatastore(ctx, store)
	}
	if err != nil {
		return fmt.Errorf("pbs verify run: %w", err)
	}
	return pbsWriteMutationResult(ctx, cmdCtx, prov, "pbs verify run", target, safety.TierReversible, upid, nil)
}

// === pbs sync run ===

func runPBSSyncRun(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "--") {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs sync run <job-id>"), app.ExitUsage)
	}
	jobID := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	runner, err := requirePBSSyncRun(prov)
	if err != nil {
		return err
	}
	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return err
	}
	items, err := jobs.PBSSyncJobs(ctx)
	if err != nil {
		return fmt.Errorf("list sync jobs: %w", err)
	}
	var job *domain.PBSSyncJob
	for i := range items {
		if items[i].ID == jobID {
			job = &items[i]
			break
		}
	}
	if job == nil {
		return app.NewExitError(fmt.Errorf("sync job %q not found", jobID), app.ExitNotFound)
	}

	var warnings []string
	if job.RemoveVanished {
		// A remove-vanished sync can delete local snapshots that no longer
		// exist on the source: escalate to type-in confirmation.
		warnings = append(warnings, "sync job has remove-vanished enabled: snapshots missing on the source will be deleted locally")
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", warnings[0])
		if err := checkDestructive(cmdCtx, fmt.Sprintf("pbs sync run %s (remove-vanished)", jobID), jobID); err != nil {
			return err
		}
	} else {
		if err := checkDisruptive(cmdCtx, fmt.Sprintf("pbs sync run %s", jobID)); err != nil {
			return err
		}
	}

	if err := pbsCheckConflictingTasks(ctx, prov, job.Store); err != nil {
		return err
	}

	upid, err := runner.PBSRunSyncJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("pbs sync run: %w", err)
	}
	return pbsWriteMutationResult(ctx, cmdCtx, prov, "pbs sync run", jobID, safety.TierDisruptive, upid, warnings)
}

// === pbs prune run ===

func runPBSPruneRun(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "--") {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs prune run <job-id>"), app.ExitUsage)
	}
	jobID := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	runner, err := requirePBSPruneRun(prov)
	if err != nil {
		return err
	}
	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return err
	}
	items, err := jobs.PBSPruneJobs(ctx)
	if err != nil {
		return fmt.Errorf("list prune jobs: %w", err)
	}
	var job *domain.PBSPruneJob
	for i := range items {
		if items[i].ID == jobID {
			job = &items[i]
			break
		}
	}
	if job == nil {
		return app.NewExitError(fmt.Errorf("prune job %q not found", jobID), app.ExitNotFound)
	}

	// Pruning permanently removes backup snapshots: destructive tier with
	// typed confirmation of the job ID.
	if err := checkDestructive(cmdCtx, fmt.Sprintf("pbs prune run %s (removes snapshots on datastore %q)", jobID, job.Store), jobID); err != nil {
		return err
	}
	if err := pbsCheckConflictingTasks(ctx, prov, job.Store); err != nil {
		return err
	}

	upid, err := runner.PBSRunPruneJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("pbs prune run: %w", err)
	}
	return pbsWriteMutationResult(ctx, cmdCtx, prov, "pbs prune run", jobID, safety.TierDestructive, upid, nil)
}

// === pbs garbage-collection run ===

func runPBSGCRun(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "--") {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs garbage-collection run <datastore>"), app.ExitUsage)
	}
	store := args[0]

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	runner, err := requirePBSGCRun(prov)
	if err != nil {
		return err
	}

	if err := checkDisruptive(cmdCtx, fmt.Sprintf("pbs garbage-collection run %s (permanently frees unreferenced chunks)", store)); err != nil {
		return err
	}
	if err := pbsCheckConflictingTasks(ctx, prov, store); err != nil {
		return err
	}

	upid, err := runner.PBSRunGarbageCollection(ctx, store)
	if err != nil {
		return fmt.Errorf("pbs garbage-collection run: %w", err)
	}
	return pbsWriteMutationResult(ctx, cmdCtx, prov, "pbs garbage-collection run", store, safety.TierDisruptive, upid, nil)
}
