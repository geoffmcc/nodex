package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// Handlers for the `nodex pbs` command group (Proxmox Backup Server,
// read-only). Every handler connects the profile's provider and requires the
// matching PBS capability interface, so running a pbs command against a
// non-PBS profile fails with an unsupported-capability error.

func formatEpoch(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

// === pbs status ===

func runPBSStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs status"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sys, err := requirePBSSystem(prov)
	if err != nil {
		return err
	}
	status, err := sys.PBSNodeStatus(ctx)
	if err != nil {
		return fmt.Errorf("get pbs status: %w", err)
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, status)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, status)
	default:
		fmt.Fprintf(cmdCtx.Writer, "CPU:        %.1f%%\n", status.CPU*100)
		fmt.Fprintf(cmdCtx.Writer, "IO wait:    %.1f%%\n", status.Wait*100)
		fmt.Fprintf(cmdCtx.Writer, "Memory:     %s / %s\n", formatBytes(status.MemoryUsed), formatBytes(status.MemoryTotal))
		fmt.Fprintf(cmdCtx.Writer, "Swap:       %s / %s\n", formatBytes(status.SwapUsed), formatBytes(status.SwapTotal))
		fmt.Fprintf(cmdCtx.Writer, "Root FS:    %s / %s\n", formatBytes(status.RootUsed), formatBytes(status.RootTotal))
		fmt.Fprintf(cmdCtx.Writer, "Uptime:     %s\n", (time.Duration(status.Uptime) * time.Second).String())
		fmt.Fprintf(cmdCtx.Writer, "Kernel:     %s\n", status.KernelVersion)
		if status.CPUModel != "" {
			fmt.Fprintf(cmdCtx.Writer, "CPU model:  %s (%d cores)\n", status.CPUModel, status.CPUs)
		}
		if status.BootMode != "" {
			fmt.Fprintf(cmdCtx.Writer, "Boot mode:  %s\n", status.BootMode)
		}
		return nil
	}
}

// === pbs version ===

func runPBSVersion(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs version"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sys, err := requirePBSSystem(prov)
	if err != nil {
		return err
	}
	v, err := sys.PBSVersionInfo(ctx)
	if err != nil {
		return fmt.Errorf("get pbs version: %w", err)
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, v)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, v)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Proxmox Backup Server %s (release %s)\n", v.Version, v.Release)
		if v.RepoID != "" {
			fmt.Fprintf(cmdCtx.Writer, "Repository: %s\n", v.RepoID)
		}
		return nil
	}
}

// === pbs subscription ===

func runPBSSubscription(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs subscription"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sys, err := requirePBSSystem(prov)
	if err != nil {
		return err
	}
	sub, err := sys.PBSSubscription(ctx)
	if err != nil {
		return fmt.Errorf("get pbs subscription: %w", err)
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, sub)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, sub)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Status:   %s\n", sub.Status)
		if sub.ProductName != "" {
			fmt.Fprintf(cmdCtx.Writer, "Product:  %s\n", sub.ProductName)
		}
		if sub.Message != "" {
			fmt.Fprintf(cmdCtx.Writer, "Message:  %s\n", sub.Message)
		}
		if sub.NextDueDate != "" {
			fmt.Fprintf(cmdCtx.Writer, "Due date: %s\n", sub.NextDueDate)
		}
		return nil
	}
}

// === pbs certificates ===

func runPBSCertificates(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs certificates"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	sys, err := requirePBSSystem(prov)
	if err != nil {
		return err
	}
	certs, err := sys.PBSCertificates(ctx)
	if err != nil {
		return fmt.Errorf("list pbs certificates: %w", err)
	}
	if certs == nil {
		certs = []domain.PBSCertificate{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, certs)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, certs)
	default:
		headers := []string{"FILENAME", "SUBJECT", "ISSUER", "NOT-AFTER", "FINGERPRINT"}
		rows := make([][]string, 0, len(certs))
		for _, c := range certs {
			rows = append(rows, []string{
				c.Filename, c.Subject, c.Issuer, formatEpoch(c.NotAfter), c.Fingerprint,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === pbs datastore ===

func runPBSDatastoreDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs datastore <list|show> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runPBSDatastoreList(ctx, cmdCtx, args[1:])
	case "show":
		return runPBSDatastoreShow(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSDatastoreList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs datastore list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ds, err := requirePBSDatastores(prov)
	if err != nil {
		return err
	}
	stores, err := ds.PBSDatastores(ctx)
	if err != nil {
		return fmt.Errorf("list pbs datastores: %w", err)
	}
	stores = applyLimit(stores, cmdCtx.Opts.Limit)
	if stores == nil {
		stores = []domain.PBSDatastore{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, stores)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, stores)
	default:
		headers := []string{"NAME", "PATH", "GC-SCHEDULE", "PRUNE-SCHEDULE", "MAINTENANCE", "COMMENT"}
		rows := make([][]string, 0, len(stores))
		for _, s := range stores {
			rows = append(rows, []string{
				s.Name, s.Path, s.GCSchedule, s.PruneSchedule, s.MaintenanceMode, s.Comment,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// pbsDatastoreDetail is the combined config + usage output of
// `pbs datastore show`.
type pbsDatastoreDetail struct {
	domain.PBSDatastore `yaml:",inline"`
	Status              *domain.PBSDatastoreStatus `json:"status,omitempty" yaml:"status,omitempty"`
	StatusError         string                     `json:"status_error,omitempty" yaml:"status_error,omitempty"`
}

func runPBSDatastoreShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs datastore show <datastore>"), app.ExitUsage)
	}
	name := args[0]
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ds, err := requirePBSDatastores(prov)
	if err != nil {
		return err
	}
	store, err := ds.PBSDatastore(ctx, name)
	if err != nil {
		return fmt.Errorf("get pbs datastore %q: %w", name, err)
	}

	detail := pbsDatastoreDetail{PBSDatastore: *store}
	// Usage is best-effort: an unavailable (e.g. unmounted) datastore still
	// has inspectable configuration. The error is reported, not masked.
	if status, statusErr := ds.PBSDatastoreStatus(ctx, name); statusErr != nil {
		detail.StatusError = statusErr.Error()
	} else {
		detail.Status = status
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, detail)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, detail)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Name:            %s\n", detail.Name)
		fmt.Fprintf(cmdCtx.Writer, "Path:            %s\n", detail.Path)
		if detail.Comment != "" {
			fmt.Fprintf(cmdCtx.Writer, "Comment:         %s\n", detail.Comment)
		}
		if detail.GCSchedule != "" {
			fmt.Fprintf(cmdCtx.Writer, "GC schedule:     %s\n", detail.GCSchedule)
		}
		if detail.PruneSchedule != "" {
			fmt.Fprintf(cmdCtx.Writer, "Prune schedule:  %s\n", detail.PruneSchedule)
		}
		if detail.MaintenanceMode != "" {
			fmt.Fprintf(cmdCtx.Writer, "Maintenance:     %s\n", detail.MaintenanceMode)
		}
		fmt.Fprintf(cmdCtx.Writer, "Verify new:      %t\n", detail.VerifyNew)
		if detail.Status != nil {
			fmt.Fprintf(cmdCtx.Writer, "Usage:           %s / %s (%s available)\n",
				formatBytes(detail.Status.Used), formatBytes(detail.Status.Total), formatBytes(detail.Status.Avail))
		}
		if detail.StatusError != "" {
			fmt.Fprintf(cmdCtx.Writer, "Status error:    %s\n", detail.StatusError)
		}
		return nil
	}
}

// === pbs snapshot ===

func runPBSSnapshotDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs snapshot list --datastore <store> [--namespace <ns>] [--backup-type vm|ct|host] [--backup-id <id>]")
	if len(args) < 1 || args[0] != "list" {
		return app.NewExitError(usage, app.ExitUsage)
	}
	return runPBSSnapshotList(ctx, cmdCtx, args[1:])
}

func parsePBSSnapshotListArgs(args []string) (store string, filter domain.PBSSnapshotFilter, err error) {
	usage := fmt.Errorf("usage: nodex pbs snapshot list --datastore <store> [--namespace <ns>] [--backup-type vm|ct|host] [--backup-id <id>]")
	setters := map[string]*string{
		"--datastore":   &store,
		"--namespace":   &filter.Namespace,
		"--backup-type": &filter.BackupType,
		"--backup-id":   &filter.BackupID,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		flag, value := arg, ""
		hasValue := false
		if eq := strings.Index(arg, "="); eq > 0 {
			flag, value, hasValue = arg[:eq], arg[eq+1:], true
		}
		target, known := setters[flag]
		if !known {
			return "", domain.PBSSnapshotFilter{}, usage
		}
		if !hasValue {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", domain.PBSSnapshotFilter{}, usage
			}
			value = args[i+1]
			i++
		}
		if value == "" {
			return "", domain.PBSSnapshotFilter{}, usage
		}
		*target = value
	}
	if store == "" {
		return "", domain.PBSSnapshotFilter{}, usage
	}
	if filter.BackupType != "" && filter.BackupType != "vm" && filter.BackupType != "ct" && filter.BackupType != "host" {
		return "", domain.PBSSnapshotFilter{}, fmt.Errorf("invalid --backup-type %q (expected vm, ct, or host)", filter.BackupType)
	}
	return store, filter, nil
}

func runPBSSnapshotList(ctx context.Context, cmdCtx *Context, args []string) error {
	store, filter, err := parsePBSSnapshotListArgs(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	snapProv, err := requirePBSSnapshots(prov)
	if err != nil {
		return err
	}
	snaps, err := snapProv.PBSSnapshots(ctx, store, filter)
	if err != nil {
		return fmt.Errorf("list pbs snapshots: %w", err)
	}
	snaps = applyLimit(snaps, cmdCtx.Opts.Limit)
	if snaps == nil {
		snaps = []domain.PBSSnapshot{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, snaps)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, snaps)
	default:
		headers := []string{"TYPE", "ID", "TIME", "SIZE", "OWNER", "PROTECTED", "VERIFIED"}
		rows := make([][]string, 0, len(snaps))
		for _, s := range snaps {
			verified := ""
			if s.Verification != nil {
				verified = s.Verification.State
			}
			protected := ""
			if s.Protected {
				protected = "yes"
			}
			rows = append(rows, []string{
				s.BackupType, s.BackupID, formatEpoch(s.BackupTime),
				formatBytes(s.Size), s.Owner, protected, verified,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === pbs task ===

func runPBSTaskDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs task <list|show|log> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runPBSTaskList(ctx, cmdCtx, args[1:])
	case "show":
		return runPBSTaskShow(ctx, cmdCtx, args[1:])
	case "log":
		return runPBSTaskLog(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSTaskList(ctx context.Context, cmdCtx *Context, args []string) error {
	filter := domain.PBSTaskFilter{}
	for _, arg := range args {
		switch arg {
		case "--running":
			filter.Running = true
		case "--errors":
			filter.Errors = true
		default:
			return app.NewExitError(
				fmt.Errorf("usage: nodex pbs task list [--running] [--errors]"),
				app.ExitUsage,
			)
		}
	}
	if cmdCtx.Opts.Limit > 0 {
		filter.Limit = int64(cmdCtx.Opts.Limit)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	taskProv, err := requirePBSTasks(prov)
	if err != nil {
		return err
	}
	tasks, err := taskProv.PBSTasks(ctx, filter)
	if err != nil {
		return fmt.Errorf("list pbs tasks: %w", err)
	}
	tasks = applyLimit(tasks, cmdCtx.Opts.Limit)
	if tasks == nil {
		tasks = []domain.PBSTask{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, tasks)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, tasks)
	default:
		headers := []string{"UPID", "TYPE", "ID", "USER", "START", "STATUS"}
		rows := make([][]string, 0, len(tasks))
		for _, t := range tasks {
			rows = append(rows, []string{
				t.UPID, t.WorkerType, t.WorkerID, t.User, formatEpoch(t.StartTime), t.Status,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runPBSTaskShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs task show <upid>"), app.ExitUsage)
	}
	upid := args[0]
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	taskProv, err := requirePBSTasks(prov)
	if err != nil {
		return err
	}
	status, err := taskProv.PBSTaskStatus(ctx, upid)
	if err != nil {
		return fmt.Errorf("get pbs task: %w", err)
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, status)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, status)
	default:
		fmt.Fprintf(cmdCtx.Writer, "UPID:        %s\n", status.UPID)
		fmt.Fprintf(cmdCtx.Writer, "Type:        %s\n", status.WorkerType)
		if status.WorkerID != "" {
			fmt.Fprintf(cmdCtx.Writer, "Worker ID:   %s\n", status.WorkerID)
		}
		fmt.Fprintf(cmdCtx.Writer, "User:        %s\n", status.User)
		fmt.Fprintf(cmdCtx.Writer, "Node:        %s\n", status.Node)
		fmt.Fprintf(cmdCtx.Writer, "Started:     %s\n", formatEpoch(status.StartTime))
		if status.EndTime != 0 {
			fmt.Fprintf(cmdCtx.Writer, "Ended:       %s\n", formatEpoch(status.EndTime))
		}
		fmt.Fprintf(cmdCtx.Writer, "Status:      %s\n", status.Status)
		if status.ExitStatus != "" {
			fmt.Fprintf(cmdCtx.Writer, "Exit status: %s\n", status.ExitStatus)
		}
		return nil
	}
}

func runPBSTaskLog(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs task log <upid>"), app.ExitUsage)
	}
	upid := args[0]
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	taskProv, err := requirePBSTasks(prov)
	if err != nil {
		return err
	}
	lines, err := taskProv.PBSTaskLog(ctx, upid)
	if err != nil {
		return fmt.Errorf("get pbs task log: %w", err)
	}
	lines = applyLimit(lines, cmdCtx.Opts.Limit)
	if lines == nil {
		lines = []domain.PBSTaskLogLine{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, lines)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, lines)
	default:
		for _, l := range lines {
			fmt.Fprintf(cmdCtx.Writer, "%s\n", l.Text)
		}
		return nil
	}
}

// === pbs verify / prune / sync ===

func runPBSVerifyDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs verify <list|run> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runPBSVerifyList(ctx, cmdCtx, args[1:])
	case "run":
		return runPBSVerifyRun(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSVerifyList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs verify list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return err
	}
	items, err := jobs.PBSVerifyJobs(ctx)
	if err != nil {
		return fmt.Errorf("list pbs verify jobs: %w", err)
	}
	items = applyLimit(items, cmdCtx.Opts.Limit)
	if items == nil {
		items = []domain.PBSVerifyJob{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, items)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, items)
	default:
		headers := []string{"ID", "DATASTORE", "NAMESPACE", "SCHEDULE", "OUTDATED-AFTER", "COMMENT"}
		rows := make([][]string, 0, len(items))
		for _, j := range items {
			outdated := ""
			if j.OutdatedAfter > 0 {
				outdated = strconv.FormatInt(j.OutdatedAfter, 10) + "d"
			}
			rows = append(rows, []string{j.ID, j.Store, j.Namespace, j.Schedule, outdated, j.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runPBSPruneDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs prune <list|run> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runPBSPruneList(ctx, cmdCtx, args[1:])
	case "run":
		return runPBSPruneRun(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSPruneList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs prune list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return err
	}
	items, err := jobs.PBSPruneJobs(ctx)
	if err != nil {
		return fmt.Errorf("list pbs prune jobs: %w", err)
	}
	items = applyLimit(items, cmdCtx.Opts.Limit)
	if items == nil {
		items = []domain.PBSPruneJob{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, items)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, items)
	default:
		headers := []string{"ID", "DATASTORE", "NAMESPACE", "SCHEDULE", "DISABLED", "KEEP", "COMMENT"}
		rows := make([][]string, 0, len(items))
		for _, j := range items {
			disabled := ""
			if j.Disable {
				disabled = "yes"
			}
			keep := formatPruneKeep(j)
			rows = append(rows, []string{j.ID, j.Store, j.Namespace, j.Schedule, disabled, keep, j.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func formatPruneKeep(j domain.PBSPruneJob) string {
	parts := []string{}
	add := func(label string, v int64) {
		if v > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", label, v))
		}
	}
	add("last", j.KeepLast)
	add("hourly", j.KeepHourly)
	add("daily", j.KeepDaily)
	add("weekly", j.KeepWeekly)
	add("monthly", j.KeepMonthly)
	add("yearly", j.KeepYearly)
	return strings.Join(parts, ",")
}

func runPBSSyncDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs sync <list|run> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runPBSSyncList(ctx, cmdCtx, args[1:])
	case "run":
		return runPBSSyncRun(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSSyncList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex pbs sync list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	jobs, err := requirePBSJobs(prov)
	if err != nil {
		return err
	}
	items, err := jobs.PBSSyncJobs(ctx)
	if err != nil {
		return fmt.Errorf("list pbs sync jobs: %w", err)
	}
	items = applyLimit(items, cmdCtx.Opts.Limit)
	if items == nil {
		items = []domain.PBSSyncJob{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, items)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, items)
	default:
		headers := []string{"ID", "DATASTORE", "REMOTE", "REMOTE-STORE", "DIRECTION", "SCHEDULE", "COMMENT"}
		rows := make([][]string, 0, len(items))
		for _, j := range items {
			rows = append(rows, []string{
				j.ID, j.Store, j.Remote, j.RemoteStore, j.SyncDirection, j.Schedule, j.Comment,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === pbs garbage-collection ===

func runPBSGCDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs garbage-collection <status|run> [args]")
	if len(args) < 1 {
		return app.NewExitError(usage, app.ExitUsage)
	}
	switch args[0] {
	case "status":
		return runPBSGCStatus(ctx, cmdCtx, args[1:])
	case "run":
		return runPBSGCRun(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(usage, app.ExitUsage)
	}
}

func runPBSGCStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex pbs garbage-collection status [--datastore <store>]")
	store := ""
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
		default:
			return app.NewExitError(usage, app.ExitUsage)
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	gcProv, err := requirePBSGC(prov)
	if err != nil {
		return err
	}

	var statuses []domain.PBSGCStatus
	if store != "" {
		status, err := gcProv.PBSGCStatus(ctx, store)
		if err != nil {
			return fmt.Errorf("get pbs garbage-collection status: %w", err)
		}
		statuses = []domain.PBSGCStatus{*status}
	} else {
		statuses, err = gcProv.PBSGCStatuses(ctx)
		if err != nil {
			return fmt.Errorf("list pbs garbage-collection status: %w", err)
		}
	}
	statuses = applyLimit(statuses, cmdCtx.Opts.Limit)
	if statuses == nil {
		statuses = []domain.PBSGCStatus{}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, statuses)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, statuses)
	default:
		headers := []string{"DATASTORE", "SCHEDULE", "LAST-RUN", "STATE", "NEXT-RUN", "REMOVED", "PENDING"}
		rows := make([][]string, 0, len(statuses))
		for _, g := range statuses {
			rows = append(rows, []string{
				g.Store, g.Schedule, formatEpoch(g.LastRunEndtime), g.LastRunState,
				formatEpoch(g.NextRun), formatBytes(g.RemovedBytes), formatBytes(g.PendingBytes),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
