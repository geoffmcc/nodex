package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/pathvalidate"
	"github.com/geoffmcc/nodex/internal/safety"
)

// --- requireXxx helpers for Phase 4 ---

func requireBackupMutation(prov domain.Provider) (domain.BackupMutationProvider, error) {
	p, ok := prov.(domain.BackupMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: backup mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

func requireStorageMutation(prov domain.Provider) (domain.StorageMutationProvider, error) {
	p, ok := prov.(domain.StorageMutationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: storage mutation commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

func requireMigration(prov domain.Provider) (domain.MigrationProvider, error) {
	p, ok := prov.(domain.MigrationProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: migration commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

func requireClone(prov domain.Provider) (domain.CloneProvider, error) {
	p, ok := prov.(domain.CloneProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: clone commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

func requireDisk(prov domain.Provider) (domain.DiskProvider, error) {
	p, ok := prov.(domain.DiskProvider)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: disk commands not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return p, nil
}

// checkDisruptive verifies Tier 2 authorization. Returns nil if authorized.
// If not authorized, returns a descriptive error. Prints warnings and prompts
// to stderr when interactive confirmation is required but not provided.
func checkDisruptive(cmdCtx *Context, desc string) error {
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if !result.ConfirmationRequired {
		return nil // Authorized via flags.
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

// checkDestructive verifies Tier 3 authorization with type-in confirmation.
// Returns nil if authorized. If --yes --force are provided but type-in is needed,
// reads confirmation from cmdCtx.Stdin. Returns error if not authorized.
func checkDestructive(cmdCtx *Context, desc, target string) error {
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDestructive,
		ResourceDescription: desc,
		RequiresTypeConfirm: true,
		TypeConfirmTarget:   target,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if !result.ConfirmationRequired {
		return nil // Authorized (all conditions met).
	}
	if cmdCtx.Opts.NonInteractive {
		return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
	}
	if result.Warning != "" {
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
	}
	if result.TypeConfirmRequired && !result.DoubleConfirmRequired {
		// --yes --force passed, now need type-in confirmation.
		fmt.Fprintf(cmdCtx.ErrW, "%s", result.Message)
		reader := bufio.NewReader(cmdCtx.Stdin)
		typed, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		typed = strings.TrimSpace(typed)
		if typed != target {
			return app.NewExitError(
				fmt.Errorf("%w: typed %q does not match target %q", safety.ErrTypeConfirmMismatch, typed, target),
				app.ExitUsage,
			)
		}
		return nil // Type-in confirmed.
	}
	// Confirmation required but flags not provided.
	fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
	return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
}

// --- Backup Create (Tier 2: disruptive) ---
// nodex backup create <node>/<vmid> <storage> <mode>

func runBackupCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 3 || len(args) > 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup create <node>/<vmid> <storage> <mode>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	storage := args[1]
	mode := args[2]

	if mode != "snapshot" && mode != "suspend" && mode != "stop" {
		return app.NewExitError(fmt.Errorf("mode must be snapshot, suspend, or stop"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("backup VM %s/%d to storage %s (mode: %s)", node, vmid, storage, mode)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := bp.CreateBackup(ctx, node, vmid, storage, mode)
	if err != nil {
		return fmt.Errorf("create backup %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "backup create", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- Backup Restore (Tier 2: disruptive) ---
// nodex backup restore <node> <new-vmid> <archive> [storage]

func runBackupRestore(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 3 || len(args) > 4 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup restore <node> <new-vmid> <archive> [storage]"), app.ExitUsage)
	}

	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}

	vmid, err := strconv.Atoi(args[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid new VMID %q", args[1]), app.ExitUsage)
	}

	archive := args[2]
	storage := ""
	if len(args) == 4 {
		storage = args[3]
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("restore VM %d from archive %s on node %s", vmid, archive, node)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := bp.RestoreVM(ctx, node, vmid, archive, storage)
	if err != nil {
		return fmt.Errorf("restore VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "backup restore", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- Backup Job List (Tier 0: read) ---
// nodex backup job list

func runBackupJobList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup job list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	schedules, err := bp.GetBackupSchedules(ctx)
	if err != nil {
		return fmt.Errorf("list backup schedules: %w", err)
	}

	return writeBackupSchedules(cmdCtx, applyLimit(schedules, cmdCtx.Opts.Limit))
}

// --- Backup Job Show (Tier 0: read) ---
// nodex backup job show <id>

func runBackupJobShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup job show <id>"), app.ExitUsage)
	}

	id := args[0]
	if id == "" {
		return app.NewExitError(fmt.Errorf("backup schedule ID is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	schedule, err := bp.GetBackupSchedule(ctx, id)
	if err != nil {
		return fmt.Errorf("get backup schedule %s: %w", id, err)
	}

	return writeBackupSchedules(cmdCtx, []domain.BackupSchedule{*schedule})
}

// --- Backup Job Create (Tier 2: disruptive) ---
// nodex backup job create storage=<name> mode=<mode> starttime=<HH:MM> [node=<n>] [vmid=<id>] [dow=<days>] ...

func runBackupJobCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup job create storage=<name> mode=<mode> starttime=<HH:MM> [node=<n>] [vmid=<id>] [dow=<d>] [compress=<c>] [comment=<c>] [mailnotification=<m>] [mailto=<m>] [prune-backups=<p>] [pool=<p>]"), app.ExitUsage)
	}

	params, err := parseBackupScheduleArgs(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	if params.Storage == "" || params.Mode == "" || params.Starttime == "" {
		return app.NewExitError(fmt.Errorf("storage, mode, and starttime are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("backup job on storage %s (mode: %s, start: %s)", params.Storage, params.Mode, params.Starttime)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := bp.CreateBackupSchedule(ctx, params)
	if err != nil {
		return fmt.Errorf("create backup schedule: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "%s\n", upid)
	return nil
}

// --- Backup Job Update (Tier 2: disruptive) ---
// nodex backup job update <id> [key=value ...]

func runBackupJobUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup job update <id> [key=value ...]"), app.ExitUsage)
	}

	id := args[0]
	if id == "" {
		return app.NewExitError(fmt.Errorf("backup schedule ID is required"), app.ExitUsage)
	}

	params, err := parseBackupScheduleArgs(args[1:])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("backup job %s", id)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := bp.UpdateBackupSchedule(ctx, id, params); err != nil {
		return fmt.Errorf("update backup schedule %s: %w", id, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Backup schedule %s updated\n", id)
	return nil
}

// --- Backup Job Delete (Tier 3: destructive) ---
// nodex backup job delete <id>

func runBackupJobDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup job delete <id>"), app.ExitUsage)
	}

	id := args[0]
	if id == "" {
		return app.NewExitError(fmt.Errorf("backup schedule ID is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	bp, err := requireBackupMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("backup job %s", id)
	if err := checkDestructive(cmdCtx, desc, id); err != nil {
		return err
	}
	// Check if confirmation was obtained

	if err := bp.DeleteBackupSchedule(ctx, id); err != nil {
		return fmt.Errorf("delete backup schedule %s: %w", id, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Backup schedule %s deleted\n", id)
	return nil
}

// --- Backup Job Dispatch ---

func runBackupJobDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex backup job <list|show|create|update|delete> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  list          List all backup schedules")
		fmt.Fprintln(cmdCtx.Writer, "  show <id>     Show a backup schedule")
		fmt.Fprintln(cmdCtx.Writer, "  create        Create a backup schedule (key=value ...)")
		fmt.Fprintln(cmdCtx.Writer, "  update <id>   Update a backup schedule (key=value ...)")
		fmt.Fprintln(cmdCtx.Writer, "  delete <id>   Delete a backup schedule (destructive)")
		return nil
	}
	switch args[0] {
	case "list":
		return runBackupJobList(ctx, cmdCtx, args[1:])
	case "show":
		return runBackupJobShow(ctx, cmdCtx, args[1:])
	case "create":
		return runBackupJobCreate(ctx, cmdCtx, args[1:])
	case "update":
		return runBackupJobUpdate(ctx, cmdCtx, args[1:])
	case "delete":
		return runBackupJobDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown backup job subcommand: %s (use list, show, create, update, or delete)", args[0]),
			app.ExitUsage,
		)
	}
}

// --- Storage Upload (Tier 2: disruptive) ---
// nodex storage upload <node> <storage> <local-file>

func runStorageUpload(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage upload <node> <storage> <local-file>"), app.ExitUsage)
	}

	node := args[0]
	storage := args[1]
	localPath := filepath.Clean(args[2])

	if node == "" || storage == "" || localPath == "" {
		return app.NewExitError(fmt.Errorf("node, storage, and local-file are required"), app.ExitUsage)
	}

	// Validate the local file is safe to open.
	if err := pathvalidate.RejectNonRegular(localPath); err != nil {
		return app.NewExitError(fmt.Errorf("local file: %w", err), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireStorageMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("upload %s to %s/%s", localPath, node, storage)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := sp.UploadContent(ctx, node, storage, localPath)
	if err != nil {
		return fmt.Errorf("upload to %s/%s: %w", node, storage, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "storage upload", fmt.Sprintf("%s/%s", node, storage), "disruptive")
}

// --- Storage Download (Tier 1: read, reversible in effect) ---
// nodex storage download <node> <storage> <volume-id> <local-path>

func runStorageDownload(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 4 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage download <node> <storage> <volume-id> <local-path>"), app.ExitUsage)
	}

	node := args[0]
	storage := args[1]
	volumeID := args[2]
	localPath := args[3]

	if node == "" || storage == "" || volumeID == "" || localPath == "" {
		return app.NewExitError(fmt.Errorf("all arguments are required"), app.ExitUsage)
	}

	// Clean and validate the destination path.
	cleaned := filepath.Clean(localPath)
	if err := pathvalidate.ValidateSafePath(cleaned); err != nil {
		return app.NewExitError(fmt.Errorf("unsafe destination path %s: %w", localPath, err), app.ExitUsage)
	}

	// Check for overwrite safety using Lstat (does not follow symlinks).
	if !cmdCtx.Opts.Force {
		if info, err := os.Lstat(cleaned); err == nil {
			if info.Mode().IsRegular() {
				return app.NewExitError(
					fmt.Errorf("destination %s already exists; use --force to overwrite", cleaned),
					app.ExitUsage,
				)
			}
			// Non-regular existing path (dir, symlink, etc.) — refuse regardless.
			return app.NewExitError(
				fmt.Errorf("destination %s exists and is not a regular file", cleaned),
				app.ExitUsage,
			)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat destination: %w", err)
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireStorageMutation(prov)
	if err != nil {
		return err
	}

	// Write to a temporary file in the destination directory, then atomically rename.
	// This ensures no partial or corrupt file is left at the final path.
	dir := filepath.Dir(cleaned)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".nodex-tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := sp.DownloadContentBody(ctx, node, storage, volumeID, tmp); err != nil {
		cleanupTmp()
		return fmt.Errorf("download %s/%s/%s: %w", node, storage, volumeID, err)
	}

	// Sync to disk before closing to ensure durability.
	if err := tmp.Sync(); err != nil {
		cleanupTmp()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return fmt.Errorf("close temporary file: %w", err)
	}

	// On Windows, os.Rename fails if the target exists and is open, so remove target first.
	if cmdCtx.Opts.Force {
		if info, err := os.Lstat(cleaned); err == nil {
			if info.Mode().IsRegular() {
				if err := os.Remove(cleaned); err != nil {
					cleanupTmp()
					return fmt.Errorf("remove existing file %s: %w", cleaned, err)
				}
			}
		}
	}

	if err := os.Rename(tmpPath, cleaned); err != nil {
		cleanupTmp()
		return fmt.Errorf("finalize download: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Downloaded %s/%s/%s to %s\n", node, storage, volumeID, cleaned)
	return nil
}

// --- Storage Delete (Tier 3: destructive) ---
// nodex storage delete <node> <storage> <volume-id>

func runStorageDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex storage delete <node> <storage> <volume-id>"), app.ExitUsage)
	}

	node := args[0]
	storage := args[1]
	volumeID := args[2]

	if node == "" || storage == "" || volumeID == "" {
		return app.NewExitError(fmt.Errorf("all arguments are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sp, err := requireStorageMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("storage volume %s/%s/%s", node, storage, volumeID)
	if err := checkDestructive(cmdCtx, desc, volumeID); err != nil {
		return err
	}

	upid, err := sp.DeleteContent(ctx, node, storage, volumeID)
	if err != nil {
		return fmt.Errorf("delete %s/%s/%s: %w", node, storage, volumeID, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "storage delete", fmt.Sprintf("%s/%s/%s", node, storage, volumeID), "destructive")
}

// --- VM Migrate (Tier 2: disruptive) ---
// nodex vm migrate <node>/<vmid> <target> [online]

func runVMMigrate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 || len(args) > 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm migrate <node>/<vmid> <target> [online]"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	target := args[1]
	online := false
	if len(args) == 3 && args[2] == "online" {
		online = true
	} else if len(args) == 3 {
		return app.NewExitError(fmt.Errorf("optional third argument must be 'online'"), app.ExitUsage)
	}

	if target == "" {
		return app.NewExitError(fmt.Errorf("target node is required"), app.ExitUsage)
	}
	if target == node {
		return app.NewExitError(fmt.Errorf("target node must differ from source node"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	mp, err := requireMigration(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d -> %s", node, vmid, target)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := mp.VMMigrate(ctx, node, vmid, target, online)
	if err != nil {
		return fmt.Errorf("migrate VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "vm migrate", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- Container Migrate (Tier 2: disruptive) ---
// nodex container migrate <node>/<vmid> <target>

func runCTMigrate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex container migrate <node>/<vmid> <target>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	target := args[1]
	if target == "" {
		return app.NewExitError(fmt.Errorf("target node is required"), app.ExitUsage)
	}
	if target == node {
		return app.NewExitError(fmt.Errorf("target node must differ from source node"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	mp, err := requireMigration(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d -> %s", node, vmid, target)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := mp.CTMigrate(ctx, node, vmid, target)
	if err != nil {
		return fmt.Errorf("migrate container %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "container migrate", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- VM Clone (Tier 2: disruptive) ---
// nodex vm clone <node>/<vmid> <new-vmid> <name> [storage]

func runVMClone(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 3 || len(args) > 4 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm clone <node>/<vmid> <new-vmid> <name> [storage]"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	newVmid, err := strconv.Atoi(args[1])
	if err != nil || newVmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid new VMID %q", args[1]), app.ExitUsage)
	}

	name := args[2]
	storage := ""
	if len(args) == 4 {
		storage = args[3]
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cp, err := requireClone(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d clone to VM %d", node, vmid, newVmid)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := cp.VMClone(ctx, node, vmid, newVmid, name, storage)
	if err != nil {
		return fmt.Errorf("clone VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "vm clone", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- Container Clone (Tier 2: disruptive) ---
// nodex container clone <node>/<vmid> <new-vmid> <name> [storage]

func runCTClone(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 3 || len(args) > 4 {
		return app.NewExitError(fmt.Errorf("usage: nodex container clone <node>/<vmid> <new-vmid> <name> [storage]"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	newVmid, err := strconv.Atoi(args[1])
	if err != nil || newVmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid new VMID %q", args[1]), app.ExitUsage)
	}

	hostname := args[2]
	storage := ""
	if len(args) == 4 {
		storage = args[3]
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	cp, err := requireClone(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("container %s/%d clone to %d", node, vmid, newVmid)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := cp.CTClone(ctx, node, vmid, newVmid, hostname, storage)
	if err != nil {
		return fmt.Errorf("clone container %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "container clone", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- VM Disk Resize (Tier 2: disruptive) ---
// nodex vm disk resize <node>/<vmid> <disk> <size>

func runVMDiskResize(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm disk resize <node>/<vmid> <disk> <size>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	disk := args[1]
	size := args[2]

	if disk == "" || size == "" {
		return app.NewExitError(fmt.Errorf("disk identifier and size are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	dp, err := requireDisk(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d disk %s resize to %s", node, vmid, disk, size)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := dp.VMDiskResize(ctx, node, vmid, disk, size)
	if err != nil {
		return fmt.Errorf("resize disk VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "vm disk resize", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- VM Disk Move (Tier 2: disruptive) ---
// nodex vm disk move <node>/<vmid> <disk> <storage>

func runVMDiskMove(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 3 {
		return app.NewExitError(fmt.Errorf("usage: nodex vm disk move <node>/<vmid> <disk> <storage>"), app.ExitUsage)
	}

	node, vmid, err := parseNodeVMID(args[0])
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	disk := args[1]
	storage := args[2]

	if disk == "" || storage == "" {
		return app.NewExitError(fmt.Errorf("disk identifier and target storage are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	dp, err := requireDisk(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("VM %s/%d disk %s move to %s", node, vmid, disk, storage)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	upid, err := dp.VMDiskMove(ctx, node, vmid, disk, storage)
	if err != nil {
		return fmt.Errorf("move disk VM %s/%d: %w", node, vmid, err)
	}

	return runMutationWithPolling(ctx, cmdCtx, prov, node, upid, "vm disk move", fmt.Sprintf("%s/%d", node, vmid), "disruptive")
}

// --- VM Disk Dispatch ---

func runVMDiskDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex vm disk <resize|move> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  resize  <node>/<vmid> <disk> <size>")
		fmt.Fprintln(cmdCtx.Writer, "  move    <node>/<vmid> <disk> <storage>")
		return nil
	}
	switch args[0] {
	case "resize":
		return runVMDiskResize(ctx, cmdCtx, args[1:])
	case "move":
		return runVMDiskMove(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown vm disk subcommand: %s (use resize or move)", args[0]),
			app.ExitUsage,
		)
	}
}

// --- Helper: parseBackupScheduleArgs parses key=value args into BackupScheduleCreateParams ---

func parseBackupScheduleArgs(args []string) (domain.BackupScheduleCreateParams, error) {
	var params domain.BackupScheduleCreateParams
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return params, fmt.Errorf("invalid parameter %q: expected key=value format", arg)
		}
		key := parts[0]
		val := parts[1]
		switch key {
		case "storage":
			params.Storage = val
		case "mode":
			params.Mode = val
		case "starttime":
			params.Starttime = val
		case "node":
			params.Node = val
		case "vmid":
			params.VMID = val
		case "dow":
			params.Dow = val
		case "compress":
			params.Compress = val
		case "comment":
			params.Comment = val
		case "mailnotification":
			params.MailNotification = val
		case "mailto":
			params.Mailto = val
		case "prune-backups":
			params.PruneBackups = val
		case "pool":
			params.Pool = val
		case "all":
			if val == "1" || val == "true" {
				params.All = 1
			}
		case "enabled":
			if val == "1" || val == "true" {
				params.Enabled = 1
			}
		case "quiet":
			if val == "1" || val == "true" {
				params.Quiet = 1
			}
		case "bwlimit":
			v, err := strconv.Atoi(val)
			if err != nil {
				return params, fmt.Errorf("invalid bwlimit value: %s", val)
			}
			params.Bwlimit = v
		case "ionice":
			v, err := strconv.Atoi(val)
			if err != nil {
				return params, fmt.Errorf("invalid ionice value: %s", val)
			}
			params.Ionice = v
		case "maxfiles":
			v, err := strconv.Atoi(val)
			if err != nil {
				return params, fmt.Errorf("invalid maxfiles value: %s", val)
			}
			params.Maxfiles = v
		default:
			return params, fmt.Errorf("unknown parameter: %s", key)
		}
	}
	return params, nil
}

// writeBackupSchedules writes backup schedule items in the configured output format.
func writeBackupSchedules(cmdCtx *Context, schedules []domain.BackupSchedule) error {
	if schedules == nil {
		schedules = []domain.BackupSchedule{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, schedules)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, schedules)
	default:
		headers := []string{"ID", "STORAGE", "MODE", "START", "DOW", "VMID", "ENABLED", "COMMENT"}
		rows := make([][]string, 0, len(schedules))
		for _, s := range schedules {
			enabled := "no"
			if s.Enabled != 0 {
				enabled = "yes"
			}
			rows = append(rows, []string{
				s.ID,
				s.Storage,
				s.Mode,
				s.Starttime,
				s.Dow,
				s.VMID,
				enabled,
				s.Comment,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
