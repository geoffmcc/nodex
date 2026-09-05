package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/backuphealth"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/maintenance"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
)

// Handlers for the `nodex maintenance` command group (Phase 5: strictly
// read-only). `inventory` lists enrolled hosts, `status` runs the read-only
// check-updates preflight through the allowlisted Ansible boundary, and
// `plan` produces an immutable, expiring, tamper-evident plan. Nothing here
// modifies a managed host.

// runCheckUpdates is the seam between the CLI and the Ansible adapter,
// replaceable in tests with canned results.
var runCheckUpdates = func(ctx context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
	det, err := ansible.Detect(ctx)
	if err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("maintenance preflight requires Ansible: %w", err),
			app.ExitIncompatibility,
		)
	}
	runner := &ansible.Runner{Exe: det.Path}
	return runner.Run(ctx, ansible.RunRequest{Operation: "check-updates", Hosts: hosts})
}

// runMaintenanceOperation is the only mutation seam. The operation ID is
// selected by the verified plan and resolved by ansible's embedded allowlist.
var runMaintenanceOperation = func(ctx context.Context, operation string, hosts []ansible.HostSpec, packages []string) (*ansible.RunResult, error) {
	det, err := ansible.Detect(ctx)
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("maintenance requires Ansible: %w", err), app.ExitIncompatibility)
	}
	return (&ansible.Runner{Exe: det.Path}).Run(ctx, ansible.RunRequest{Operation: operation, Hosts: hosts, Packages: packages})
}

// maintenanceFilters selects inventory hosts.
type maintenanceFilters struct {
	environment string
	group       string
	role        string
	hosts       []string
}

func parseMaintenanceFilters(args []string) (maintenanceFilters, []string, error) {
	var f maintenanceFilters
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		consumeValue := func(target *string) error {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return fmt.Errorf("flag %s requires a value", arg)
			}
			*target = args[i+1]
			i++
			return nil
		}
		switch arg {
		case "--environment":
			if err := consumeValue(&f.environment); err != nil {
				return f, nil, err
			}
		case "--group":
			if err := consumeValue(&f.group); err != nil {
				return f, nil, err
			}
		case "--role":
			if err := consumeValue(&f.role); err != nil {
				return f, nil, err
			}
		case "--host":
			var h string
			if err := consumeValue(&h); err != nil {
				return f, nil, err
			}
			f.hosts = append(f.hosts, h)
		default:
			rest = append(rest, arg)
		}
	}
	return f, rest, nil
}

// selectInventoryHosts applies filters to the configured inventory.
func selectInventoryHosts(cfg *config.Config, f maintenanceFilters) (map[string]config.InventoryHost, error) {
	if cfg.Inventory == nil || len(cfg.Inventory.Hosts) == 0 {
		return nil, app.NewExitError(
			fmt.Errorf("no inventory hosts configured; add an \"inventory\" section (schema version 2)"),
			app.ExitConfig,
		)
	}
	wantHost := map[string]bool{}
	for _, h := range f.hosts {
		wantHost[h] = true
	}
	selected := map[string]config.InventoryHost{}
	for name, h := range cfg.Inventory.Hosts {
		if f.environment != "" && h.Environment != f.environment {
			continue
		}
		if f.group != "" && h.MaintenanceGroup != f.group {
			continue
		}
		if f.role != "" && h.Role != f.role {
			continue
		}
		if len(wantHost) > 0 && !wantHost[name] {
			continue
		}
		selected[name] = h
	}
	for h := range wantHost {
		if _, ok := cfg.Inventory.Hosts[h]; !ok {
			return nil, app.NewExitError(
				fmt.Errorf("host %q is not enrolled in the inventory", h),
				app.ExitNotFound,
			)
		}
	}
	if len(selected) == 0 {
		return nil, app.NewExitError(
			fmt.Errorf("no inventory hosts match the given filters"),
			app.ExitNotFound,
		)
	}
	return selected, nil
}

func hostSpecs(selected map[string]config.InventoryHost) []ansible.HostSpec {
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	specs := make([]ansible.HostSpec, 0, len(names))
	for _, name := range names {
		h := selected[name]
		specs = append(specs, ansible.HostSpec{
			Name:           name,
			Address:        h.Address,
			Port:           h.SSHPort,
			User:           h.SSHUser,
			KeyFile:        h.SSHKeyFile,
			KnownHostsFile: h.KnownHostsFile,
		})
	}
	return specs
}

// === maintenance inventory ===

func runMaintenanceInventory(_ context.Context, cmdCtx *Context, args []string) error {
	f, rest, err := parseMaintenanceFilters(args)
	if err != nil || len(rest) != 0 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex maintenance inventory [--environment <env>] [--group <group>] [--role <role>] [--host <name>]"),
			app.ExitUsage,
		)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected, err := selectInventoryHosts(cfg, f)
	if err != nil {
		return err
	}

	type invEntry struct {
		Name string `json:"name" yaml:"name"`
		config.InventoryHost
	}
	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]invEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, invEntry{Name: name, InventoryHost: selected[name]})
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, entries)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, entries)
	default:
		headers := []string{"NAME", "ADDRESS", "ROLE", "PVE-NODE", "ENV", "GROUP", "CRITICALITY", "BACKUP-REQ", "AUTO-REBOOT"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{
				e.Name, e.Address, e.Role, e.PVENode, e.Environment, e.MaintenanceGroup,
				e.Criticality, boolYes(e.BackupRequired), boolYes(e.AutomaticReboot),
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func boolYes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// === maintenance status ===

// maintenanceStatusResult is the combined output of `maintenance status`.
type maintenanceStatusResult struct {
	Hosts          []maintenance.HostStatus `json:"hosts" yaml:"hosts"`
	Environment    *backuphealth.Result     `json:"environment,omitempty" yaml:"environment,omitempty"`
	PartialFailure bool                     `json:"partial_failure" yaml:"partial_failure"`
}

func runMaintenanceStatus(ctx context.Context, cmdCtx *Context, args []string) error {
	f, rest, err := parseMaintenanceFilters(args)
	if err != nil || len(rest) != 0 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex maintenance status [--environment <env>] [--group <group>] [--role <role>] [--host <name>]"),
			app.ExitUsage,
		)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected, err := selectInventoryHosts(cfg, f)
	if err != nil {
		return err
	}

	res, err := runCheckUpdates(ctx, hostSpecs(selected))
	if err != nil {
		return err
	}
	statusResult := maintenanceStatusResult{
		Hosts:          maintenance.InterpretCheckUpdates(res),
		PartialFailure: res == nil || res.PartialFailure || res.ParseError != "",
	}
	for _, host := range statusResult.Hosts {
		if !host.EvidenceComplete {
			statusResult.PartialFailure = true
			break
		}
	}

	if f.environment != "" {
		envResult, err := evaluateEnvironment(ctx, cmdCtx, cfg, f.environment, true)
		if err != nil {
			return err
		}
		statusResult.Environment = envResult
		if envResult.PartialFailure {
			statusResult.PartialFailure = true
		}
	}

	if err := writeMaintenanceStatus(cmdCtx, statusResult); err != nil {
		return err
	}
	if statusResult.PartialFailure {
		return app.NewExitError(
			fmt.Errorf("maintenance status incomplete: some hosts or checks could not be evaluated"),
			app.ExitPartialFailure,
		)
	}
	return nil
}

func writeMaintenanceStatus(cmdCtx *Context, res maintenanceStatusResult) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, res)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, res)
	default:
		headers := []string{"HOST", "REACHABLE", "UPDATES", "SECURITY", "REBOOT-REQ", "FAILED-UNITS", "ROOT-USE"}
		rows := make([][]string, 0, len(res.Hosts))
		for _, h := range res.Hosts {
			rows = append(rows, []string{
				h.Host, boolYes(h.Reachable), strconv.Itoa(len(h.PendingUpdates)),
				strconv.Itoa(len(h.SecurityUpdates)), boolYes(h.RebootRequired),
				strconv.Itoa(len(h.FailedUnits)), h.RootUsage,
			})
		}
		if err := output.WriteTable(cmdCtx.Writer, headers, rows); err != nil {
			return err
		}
		for _, h := range res.Hosts {
			for _, w := range h.Warnings {
				fmt.Fprintf(cmdCtx.Writer, "  ! %s: %s\n", h.Host, w)
			}
		}
		if res.Environment != nil {
			fmt.Fprintln(cmdCtx.Writer)
			fmt.Fprintf(cmdCtx.Writer, "Environment %s: %s (maintenance safe: %t)\n",
				res.Environment.Environment, res.Environment.Overall, res.Environment.MaintenanceSafe)
			for _, b := range res.Environment.Blockers {
				fmt.Fprintf(cmdCtx.Writer, "  - %s\n", b)
			}
		}
		return nil
	}
}

// === maintenance plan ===

func runMaintenancePlan(ctx context.Context, cmdCtx *Context, args []string) error {
	usage := fmt.Errorf("usage: nodex maintenance plan --policy security-only|approved-full-upgrade [--expires-in <duration>] [--batch-size <n>] [--environment <env>] [--group <group>] [--role <role>] [--host <name>]")
	f, rest, err := parseMaintenanceFilters(args)
	if err != nil {
		return app.NewExitError(usage, app.ExitUsage)
	}
	policy, expiresIn, batchSize := "", maintenance.DefaultPlanTTL, 3
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--policy":
			if i+1 >= len(rest) {
				return app.NewExitError(usage, app.ExitUsage)
			}
			policy = rest[i+1]
			i++
		case "--expires-in":
			if i+1 >= len(rest) {
				return app.NewExitError(usage, app.ExitUsage)
			}
			d, err := time.ParseDuration(rest[i+1])
			if err != nil || d < 10*time.Minute || d > 24*time.Hour {
				return app.NewExitError(
					fmt.Errorf("--expires-in must be a duration between 10m and 24h"),
					app.ExitUsage,
				)
			}
			expiresIn = d
			i++
		case "--batch-size":
			if i+1 >= len(rest) {
				return app.NewExitError(usage, app.ExitUsage)
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 1 || n > 10 {
				return app.NewExitError(fmt.Errorf("--batch-size must be between 1 and 10"), app.ExitUsage)
			}
			batchSize = n
			i++
		default:
			return app.NewExitError(usage, app.ExitUsage)
		}
	}
	if !maintenance.ValidPolicy(policy) {
		return app.NewExitError(usage, app.ExitUsage)
	}

	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected, err := selectInventoryHosts(cfg, f)
	if err != nil {
		return err
	}

	// Preflight through the read-only check-updates operation.
	res, err := runCheckUpdates(ctx, hostSpecs(selected))
	if err != nil {
		return err
	}
	statuses := maintenance.InterpretCheckUpdates(res)
	statusByHost := map[string]maintenance.HostStatus{}
	for _, s := range statuses {
		statusByHost[s.Host] = s
	}

	plan := maintenance.Plan{
		Schema:               maintenance.PlanSchemaVersion,
		CreatedAt:            time.Now().Unix(),
		ExpiresAt:            time.Now().Add(expiresIn).Unix(),
		Environment:          f.environment,
		Policy:               policy,
		BatchSize:            batchSize,
		RebootPolicy:         maintenance.RebootPolicyNever,
		SafetyClassification: "disruptive",
	}
	plan.Snapshot = maintenance.SafetySnapshot{Version: 1, Hosts: map[string]maintenance.HostSnapshot{}}
	planID, err := maintenance.NewPlanID()
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	plan.PlanID = planID

	maxAge := config.DefaultBackupMaxAgeHours
	if f.environment != "" {
		if env, ok := cfg.Environments[f.environment]; ok && env.BackupMaxAgeHours > 0 {
			maxAge = env.BackupMaxAgeHours
		}
	}
	plan.Backup.MaxAgeHours = maxAge

	names := make([]string, 0, len(selected))
	for name := range selected {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := selected[name]
		status, hasStatus := statusByHost[name]
		ph := maintenance.PlanHost{
			Name:            name,
			Address:         h.Address,
			Role:            h.Role,
			Environment:     h.Environment,
			PVENode:         h.PVENode,
			PVEProfile:      h.PVEProfile,
			PBSProfile:      h.PBSProfile,
			Group:           h.MaintenanceGroup,
			Criticality:     orDefault(h.Criticality, config.CriticalityStandard),
			BackupRequired:  h.BackupRequired,
			AutomaticReboot: h.AutomaticReboot,
		}
		if hasStatus {
			ph.PendingUpdates = status.PendingUpdates
			ph.SecurityUpdates = status.SecurityUpdates
			ph.RebootRequired = status.RebootRequired
			ph.Warnings = status.Warnings
			if !status.Reachable {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s is unreachable", name))
			} else if !status.Supported {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s is unsupported for maintenance", name))
			} else if !status.EvidenceComplete {
				plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s produced incomplete preflight evidence", name))
			}
			plan.Snapshot.Hosts[name] = maintenance.Snapshot(ph, status, h.SSHUser, h.SSHPort, h.SSHKeyFile != "", h.KnownHostsFile != "")
		} else {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s produced no preflight results", name))
			plan.Snapshot.Hosts[name] = maintenance.HostSnapshot{Name: name, Address: h.Address, Role: h.Role, Group: h.MaintenanceGroup, Criticality: ph.Criticality, SSHUser: h.SSHUser, SSHPort: h.SSHPort, KeyConfigured: h.SSHKeyFile != "", KnownHostsConfigured: h.KnownHostsFile != ""}
		}
		if h.BackupRequired {
			plan.Backup.RequiredHosts = append(plan.Backup.RequiredHosts, name)
		}
		plan.Hosts = append(plan.Hosts, ph)
	}
	plan.HostOrder = maintenance.OrderHosts(plan.Hosts)

	// Backup satisfaction and infrastructure snapshot come from the
	// environment's backup health; without an environment linkage the
	// backup state is unknown, which blocks by default.
	if f.environment != "" {
		envResult, err := evaluateEnvironment(ctx, cmdCtx, cfg, f.environment, true)
		if err != nil {
			return err
		}
		plan.Infra = maintenance.InfraSnapshot{
			Environment:      envResult.Environment,
			Overall:          string(envResult.Overall),
			MaintenanceSafe:  envResult.MaintenanceSafe,
			Blockers:         envResult.Blockers,
			Checks:           environmentCheckStates(envResult),
			EvidenceComplete: !envResult.PartialFailure,
		}
		plan.Snapshot.Infrastructure = plan.Infra
		plan.Backup.Satisfied = envResult.MaintenanceSafe
		plan.Backup.Detail = fmt.Sprintf("environment %s backup health: %s", f.environment, envResult.Overall)
		plan.Backup = maintenanceBackupState(plan.Backup, envResult)
		if !envResult.MaintenanceSafe {
			plan.Blockers = append(plan.Blockers, envResult.Blockers...)
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("environment %s is not safe for maintenance", f.environment))
		}
	} else if len(plan.Backup.RequiredHosts) > 0 {
		plan.Backup.Satisfied = false
		plan.Backup.Detail = "backup state unknown: no --environment given for backup-required hosts"
		plan.Blockers = append(plan.Blockers, "backup requirements cannot be verified without --environment")
	} else {
		plan.Backup.Satisfied = true
		plan.Backup.CoverageComplete = true
		plan.Backup.VerificationHealthy = true
		plan.Backup.Detail = "no hosts require backups"
		plan.Infra = maintenance.InfraSnapshot{Overall: "not-configured", MaintenanceSafe: true, EvidenceComplete: true}
	}
	plan.Snapshot.Infrastructure = plan.Infra
	plan.Snapshot.Backup = plan.Backup
	plan.Snapshot.EvidenceComplete = len(plan.Blockers) == 0 && len(plan.Snapshot.Hosts) == len(plan.Hosts) && plan.Infra.EvidenceComplete

	if res == nil || res.PartialFailure || res.ParseError != "" {
		plan.Warnings = append(plan.Warnings, "preflight was incomplete; see host warnings")
	}

	plan, err = maintenance.Finalize(plan)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}

	if err := writeMaintenancePlan(cmdCtx, plan); err != nil {
		return err
	}
	if len(plan.Blockers) > 0 {
		return app.NewExitError(
			fmt.Errorf("plan %s created with %d blocker(s); apply will refuse until they clear", plan.PlanID, len(plan.Blockers)),
			app.ExitPartialFailure,
		)
	}
	return nil
}

func environmentCheckStates(result *backuphealth.Result) map[string]string {
	states := make(map[string]string, len(result.Checks))
	for _, check := range result.Checks {
		states[check.Name] = string(check.Status)
	}
	return states
}

func maintenanceBackupState(state maintenance.BackupState, result *backuphealth.Result) maintenance.BackupState {
	if result == nil {
		state.Satisfied = false
		state.CoverageComplete = false
		state.VerificationHealthy = false
		return state
	}
	state.Satisfied = result.MaintenanceSafe
	state.CoverageComplete, state.VerificationHealthy = true, true
	for _, guest := range result.Guests {
		if guest.Status == backuphealth.StatusUnknown || guest.Status == backuphealth.StatusBlocked {
			state.CoverageComplete = false
		}
		if guest.Verification == "failed" || guest.Verification == "" || guest.Status == backuphealth.StatusUnknown || guest.Status == backuphealth.StatusBlocked {
			state.VerificationHealthy = false
		}
		if int(guest.AgeHours) > state.OldestBackupAgeHours {
			state.OldestBackupAgeHours = int(guest.AgeHours)
		}
	}
	for _, check := range result.Checks {
		if check.Name == "pbs_datastores" && check.Status != backuphealth.StatusHealthy {
			state.VerificationHealthy = false
		}
	}
	return state
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func writeMaintenancePlan(cmdCtx *Context, plan maintenance.Plan) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, plan)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, plan)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Plan:        %s\n", plan.PlanID)
		fmt.Fprintf(cmdCtx.Writer, "Policy:      %s\n", plan.Policy)
		fmt.Fprintf(cmdCtx.Writer, "Created:     %s\n", time.Unix(plan.CreatedAt, 0).UTC().Format(time.RFC3339))
		fmt.Fprintf(cmdCtx.Writer, "Expires:     %s\n", time.Unix(plan.ExpiresAt, 0).UTC().Format(time.RFC3339))
		if plan.Environment != "" {
			fmt.Fprintf(cmdCtx.Writer, "Environment: %s\n", plan.Environment)
		}
		fmt.Fprintf(cmdCtx.Writer, "Reboots:     %s\n", plan.RebootPolicy)
		fmt.Fprintf(cmdCtx.Writer, "Digest:      %s\n", plan.Digest)
		fmt.Fprintln(cmdCtx.Writer)

		headers := []string{"ORDER", "HOST", "ROLE", "CRITICALITY", "UPDATES", "SECURITY", "REBOOT-REQ", "BACKUP-REQ"}
		rows := make([][]string, 0, len(plan.Hosts))
		hostByName := map[string]maintenance.PlanHost{}
		for _, h := range plan.Hosts {
			hostByName[h.Name] = h
		}
		for i, name := range plan.HostOrder {
			h := hostByName[name]
			rows = append(rows, []string{
				strconv.Itoa(i + 1), h.Name, h.Role, h.Criticality,
				strconv.Itoa(len(h.PendingUpdates)), strconv.Itoa(len(h.SecurityUpdates)),
				boolYes(h.RebootRequired), boolYes(h.BackupRequired),
			})
		}
		if err := output.WriteTable(cmdCtx.Writer, headers, rows); err != nil {
			return err
		}
		if len(plan.Warnings) > 0 {
			fmt.Fprintln(cmdCtx.Writer)
			for _, w := range plan.Warnings {
				fmt.Fprintf(cmdCtx.Writer, "Warning: %s\n", w)
			}
		}
		if len(plan.Blockers) > 0 {
			fmt.Fprintln(cmdCtx.Writer)
			fmt.Fprintln(cmdCtx.Writer, "Blockers (apply will refuse):")
			for _, b := range plan.Blockers {
				fmt.Fprintf(cmdCtx.Writer, "  - %s\n", b)
			}
		}
		fmt.Fprintln(cmdCtx.Writer)
		fmt.Fprintln(cmdCtx.Writer, "Save the full plan with: nodex --output json maintenance plan ... > plan.json")
		return nil
	}
}

type maintenanceReport struct {
	PlanID     string                    `json:"plan_id" yaml:"plan_id"`
	PlanDigest string                    `json:"plan_digest" yaml:"plan_digest"`
	ReceiptID  string                    `json:"receipt_id" yaml:"receipt_id"`
	State      string                    `json:"state" yaml:"state"`
	Verified   bool                      `json:"verified" yaml:"verified"`
	Hosts      []maintenance.HostReceipt `json:"hosts" yaml:"hosts"`
	Error      string                    `json:"error,omitempty" yaml:"error,omitempty"`
}

type maintenanceApplyArgs struct{ planPath, receiptDir, receiptPath string }

func parseMaintenanceApplyArgs(args []string) (maintenanceApplyArgs, error) {
	parsed := maintenanceApplyArgs{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan", "--receipt-dir", "--receipt":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return parsed, fmt.Errorf("%s requires a value", args[i])
			}
			switch args[i] {
			case "--plan":
				parsed.planPath = args[i+1]
			case "--receipt-dir":
				parsed.receiptDir = args[i+1]
			case "--receipt":
				parsed.receiptPath = args[i+1]
			}
			i++
		default:
			return parsed, fmt.Errorf("unknown maintenance apply argument %q", args[i])
		}
	}
	if parsed.planPath == "" {
		return parsed, fmt.Errorf("--plan is required")
	}
	if parsed.receiptDir == "" {
		parsed.receiptDir = filepath.Join(filepath.Dir(parsed.planPath), ".nodex-receipts")
	}
	if parsed.receiptPath != "" && parsed.receiptDir != filepath.Join(filepath.Dir(parsed.planPath), ".nodex-receipts") {
		return parsed, fmt.Errorf("--receipt cannot be combined with --receipt-dir")
	}
	return parsed, nil
}

func runMaintenanceApply(ctx context.Context, cmdCtx *Context, args []string) error {
	parsed, err := parseMaintenanceApplyArgs(args)
	if err != nil {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance apply --plan <file> [--receipt-dir <dir>]: %w", err), app.ExitUsage)
	}
	planPath, receiptDir := parsed.planPath, parsed.receiptDir
	plan, err := maintenance.LoadFile(planPath, time.Now())
	if err != nil {
		return app.NewExitError(fmt.Errorf("load maintenance plan: %w", err), app.ExitValidationError)
	}
	if len(plan.Blockers) != 0 {
		return app.NewExitError(fmt.Errorf("plan %s has %d blocker(s); apply refused", plan.PlanID, len(plan.Blockers)), app.ExitValidationError)
	}
	if !plan.Infra.MaintenanceSafe || (len(plan.Backup.RequiredHosts) > 0 && !plan.Backup.Satisfied) {
		return app.NewExitError(fmt.Errorf("plan %s does not contain a positive maintenance-safety decision; apply refused", plan.PlanID), app.ExitValidationError)
	}
	resume := parsed.receiptPath != ""
	if !cmdCtx.Opts.Yes || !cmdCtx.Opts.Force {
		return app.NewExitError(fmt.Errorf("confirmation refused: apply requires --yes --force"), app.ExitUsage)
	}
	if !resume && cmdCtx.Opts.ConfirmTarget != plan.PlanID {
		return app.NewExitError(fmt.Errorf("confirmation refused: apply requires --confirm-target %s", plan.PlanID), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected := map[string]config.InventoryHost{}
	for _, ph := range plan.Hosts {
		if cfg.Inventory == nil {
			return app.NewExitError(fmt.Errorf("inventory is not configured; create a new plan"), app.ExitConflict)
		}
		h, ok := cfg.Inventory.Hosts[ph.Name]
		if !ok || !inventoryHostMatchesPlan(ph, h) {
			return app.NewExitError(fmt.Errorf("inventory changed for planned host %q; create a new plan", ph.Name), app.ExitConflict)
		}
		selected[ph.Name] = h
	}
	var receipt maintenance.Receipt
	completed := map[string]bool{}
	path := maintenance.ReceiptPath(receiptDir, plan.PlanID)
	if resume {
		path = parsed.receiptPath
	}
	applyLock, err := config.Lock(path + ".apply")
	if err != nil {
		return app.NewExitError(fmt.Errorf("lock maintenance apply: %w", err), app.ExitConflict)
	}
	defer func() { _ = config.Unlock(applyLock) }()
	if resume {
		if cmdCtx.Opts.ConfirmTarget == "" {
			return app.NewExitError(fmt.Errorf("resume requires --confirm-target <receipt-id>"), app.ExitUsage)
		}
		receipt, err = maintenance.LoadReceipt(path)
		if err != nil {
			return app.NewExitError(fmt.Errorf("load maintenance receipt: %w", err), app.ExitValidationError)
		}
		if receipt.ReceiptID != cmdCtx.Opts.ConfirmTarget {
			return app.NewExitError(fmt.Errorf("confirmation refused: receipt ID does not match"), app.ExitConflict)
		}
		if err := receipt.VerifyForPlan(plan); err != nil {
			return app.NewExitError(err, app.ExitConflict)
		}
		if receipt.State == "succeeded" || receipt.State == "abandoned" {
			return app.NewExitError(fmt.Errorf("receipt is already terminal: %s", receipt.State), app.ExitConflict)
		}
		for _, host := range receipt.Hosts {
			if findPlanHost(plan, host.Host).Name == "" {
				return app.NewExitError(fmt.Errorf("receipt contains host %q not present in the plan", host.Host), app.ExitConflict)
			}
			if host.State != "succeeded" {
				return app.NewExitError(fmt.Errorf("receipt contains non-successful host %q; review it and create a new plan", host.Host), app.ExitConflict)
			}
			completed[host.Host] = true
		}
		receipt.State, receipt.Error = "running", ""
	} else {
		receipt = maintenance.NewReceipt(plan, time.Now())
		if err := receipt.Finalize(); err != nil {
			return app.NewExitError(err, app.ExitValidationError)
		}
		if _, statErr := os.Stat(path); statErr == nil {
			if _, loadErr := maintenance.LoadReceipt(path); loadErr != nil {
				return app.NewExitError(fmt.Errorf("existing receipt is invalid; refusing to overwrite: %w", loadErr), app.ExitValidationError)
			}
			return app.NewExitError(fmt.Errorf("receipt already exists at %s; refusing to rerun without operator review", path), app.ExitConflict)
		} else if !os.IsNotExist(statErr) {
			return fmt.Errorf("inspect receipt: %w", statErr)
		}
	}
	current, err := captureMaintenanceSnapshot(ctx, cmdCtx, cfg, selected, plan)
	if err != nil {
		return app.NewExitError(fmt.Errorf("revalidate maintenance safety: %w", err), app.ExitConflict)
	}
	expected := plan.Snapshot
	if resume {
		expected = snapshotWithCompletedHosts(plan.Snapshot, current, completed)
	}
	comparison := maintenance.CompareSnapshots(expected, current)
	if comparison.Overall != maintenance.DispositionMatch {
		return app.NewExitError(fmt.Errorf("maintenance plan conflict: create a new plan; changed=%v unknown=%v blockers=%v", comparison.ChangedFields, comparison.UnknownFields, comparison.Blockers), app.ExitConflict)
	}
	var saveErr error
	if resume {
		saveErr = persistReceipt(path, &receipt)
	} else {
		saveErr = maintenance.SaveReceipt(path, receipt)
	}
	if saveErr != nil {
		return fmt.Errorf("write receipt: %w", saveErr)
	}
	op := "apply-security-updates"
	if plan.Policy == maintenance.PolicyApprovedFull {
		op = "apply-approved-updates"
	}
	standard, critical := make([]string, 0), make([]string, 0)
	for _, name := range plan.HostOrder {
		if completed[name] {
			continue
		}
		h := findPlanHost(plan, name)
		if h.Criticality == config.CriticalityCritical || h.Role == config.RolePVE || h.Role == config.RolePBS || h.Role == config.RoleDNS {
			critical = append(critical, name)
		} else {
			standard = append(standard, name)
		}
	}
	for start := 0; start < len(standard); start += plan.BatchSize {
		end := start + plan.BatchSize
		if end > len(standard) {
			end = len(standard)
		}
		if err := revalidateBeforeBatch(ctx, cmdCtx, cfg, selected, plan, completed); err != nil {
			receipt.State = "blocked"
			receipt.Error = redactError(err)
			_ = persistReceipt(path, &receipt)
			return app.NewExitError(fmt.Errorf("maintenance stopped before batch; create a new plan: %s", receipt.Error), app.ExitConflict)
		}
		batch := standard[start:end]
		if err := executeMaintenanceBatch(ctx, path, &receipt, selected, plan, op, batch, completed); err != nil {
			receipt.State = hostFailureState(err)
			receipt.Error = redactError(err)
			if saveErr := persistReceipt(path, &receipt); saveErr != nil {
				return saveErr
			}
			return app.NewExitError(fmt.Errorf("maintenance apply stopped; receipt: %s", path), app.ExitAmbiguousOutcome)
		}
	}
	for _, name := range critical {
		if err := revalidateBeforeBatch(ctx, cmdCtx, cfg, selected, plan, completed); err != nil {
			receipt.State = "blocked"
			receipt.Error = redactError(err)
			if saveErr := persistReceipt(path, &receipt); saveErr != nil {
				return saveErr
			}
			return app.NewExitError(fmt.Errorf("maintenance stopped before critical host; create a new plan: %s", receipt.Error), app.ExitConflict)
		}
		if err := executeMaintenanceBatch(ctx, path, &receipt, selected, plan, op, []string{name}, completed); err != nil {
			receipt.State = hostFailureState(err)
			receipt.Error = redactError(err)
			if saveErr := persistReceipt(path, &receipt); saveErr != nil {
				return saveErr
			}
			return app.NewExitError(fmt.Errorf("maintenance apply stopped; receipt: %s", path), app.ExitAmbiguousOutcome)
		}
	}
	if len(completed) != len(plan.Hosts) {
		receipt.State = "blocked"
		receipt.Error = "not all planned hosts were completed"
		if saveErr := persistReceipt(path, &receipt); saveErr != nil {
			return saveErr
		}
		return app.NewExitError(fmt.Errorf("maintenance incomplete; receipt: %s", path), app.ExitPartialFailure)
	}
	verification, runErr := runMaintenanceOperation(ctx, "verify-maintenance", hostSpecs(selected), nil)
	if err := applyPerHostVerification(&receipt, verification, runErr); err != nil {
		receipt.State = "failed"
		receipt.Error = redactError(err)
		if saveErr := persistReceipt(path, &receipt); saveErr != nil {
			return saveErr
		}
		return app.NewExitError(fmt.Errorf("maintenance verification failed; receipt: %s", path), app.ExitPartialFailure)
	}
	receipt.State, receipt.Error, receipt.UpdatedAt = "succeeded", "", time.Now().Unix()
	if err := receipt.Finalize(); err != nil {
		return err
	}
	if err := maintenance.SaveReceipt(path, receipt); err != nil {
		return err
	}
	return writeMaintenanceReport(cmdCtx, maintenanceReport{PlanID: plan.PlanID, PlanDigest: plan.Digest, ReceiptID: receipt.ReceiptID, State: receipt.State, Verified: true, Hosts: receipt.Hosts})
}

func findPlanHost(plan maintenance.Plan, name string) maintenance.PlanHost {
	for _, h := range plan.Hosts {
		if h.Name == name {
			return h
		}
	}
	return maintenance.PlanHost{}
}

func inventoryHostMatchesPlan(planHost maintenance.PlanHost, host config.InventoryHost) bool {
	return host.Address == planHost.Address &&
		host.Role == planHost.Role &&
		host.Environment == planHost.Environment &&
		host.PVENode == planHost.PVENode &&
		host.PVEProfile == planHost.PVEProfile &&
		host.PBSProfile == planHost.PBSProfile &&
		host.MaintenanceGroup == planHost.Group &&
		orDefault(host.Criticality, config.CriticalityStandard) == planHost.Criticality &&
		host.BackupRequired == planHost.BackupRequired &&
		host.AutomaticReboot == planHost.AutomaticReboot
}

func persistReceipt(path string, receipt *maintenance.Receipt) error {
	receipt.UpdatedAt = time.Now().Unix()
	if err := receipt.Finalize(); err != nil {
		return err
	}
	if err := maintenance.SaveReceipt(path, *receipt); err != nil {
		return fmt.Errorf("persist receipt: %w", err)
	}
	return nil
}
func redactError(err error) string {
	if err == nil {
		return ""
	}
	return redact.String(output.SanitizeTerminal(err.Error()))
}
func hostFailureState(err error) string {
	if strings.Contains(strings.ToLower(redactError(err)), "unknown") {
		return "unknown"
	}
	return "failed"
}

func executeMaintenanceBatch(ctx context.Context, path string, receipt *maintenance.Receipt, selected map[string]config.InventoryHost, plan maintenance.Plan, op string, names []string, completed map[string]bool) error {
	for _, name := range names {
		setReceiptHost(receipt, name, op, "running", nil)
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{Host: name, State: "running", At: time.Now().Unix()})
	}
	if err := persistReceipt(path, receipt); err != nil {
		return err
	}
	type outcome struct {
		name   string
		result *ansible.RunResult
		err    error
	}
	ch := make(chan outcome, len(names))
	for _, name := range names {
		name := name
		go func() {
			packages := []string{}
			if op == "apply-security-updates" {
				packages = findPlanHost(plan, name).SecurityUpdates
			}
			result, err := runMaintenanceOperation(ctx, op, hostSpecs(map[string]config.InventoryHost{name: selected[name]}), packages)
			ch <- outcome{name: name, result: result, err: err}
		}()
	}
	failed := ""
	results := make([]outcome, 0, len(names))
	for range names {
		results = append(results, <-ch)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].name < results[j].name })
	for _, result := range results {
		h := receiptHost(receipt, result.name)
		if result.err != nil {
			h.State, h.Success, h.FailuresDetail = "unknown", false, []string{redactError(result.err)}
			failed = result.name
		} else if result.result == nil || !result.result.Success {
			h.State, h.Success = "failed", false
			h.FailuresDetail = []string{"Ansible reported an unsuccessful or incomplete result"}
			if result.result != nil {
				for _, host := range result.result.Hosts {
					if host.Host == result.name {
						h.Changed, h.Failures, h.Unreachable = host.Changed, host.Failures, host.Unreachable
					}
				}
			}
			failed = result.name
		} else {
			h.State, h.Success = "succeeded", true
			if result.result != nil {
				for _, host := range result.result.Hosts {
					if host.Host == result.name {
						h.Changed, h.Failures, h.Unreachable = host.Changed, host.Failures, host.Unreachable
					}
				}
			}
			completed[result.name] = true
		}
		receipt.Events = append(receipt.Events, maintenance.ReceiptEvent{Host: result.name, State: h.State, At: time.Now().Unix()})
	}
	if err := persistReceipt(path, receipt); err != nil {
		return err
	}
	if failed != "" {
		return fmt.Errorf("host %s returned a non-successful or unknown outcome", failed)
	}
	return nil
}

func receiptHost(receipt *maintenance.Receipt, name string) *maintenance.HostReceipt {
	for i := range receipt.Hosts {
		if receipt.Hosts[i].Host == name {
			return &receipt.Hosts[i]
		}
	}
	receipt.Hosts = append(receipt.Hosts, maintenance.HostReceipt{Host: name})
	return &receipt.Hosts[len(receipt.Hosts)-1]
}
func setReceiptHost(receipt *maintenance.Receipt, name, op, state string, _ error) {
	h := receiptHost(receipt, name)
	h.Operation, h.State, h.Verification = op, state, "pending"
	h.Success, h.Changed, h.Failures, h.Unreachable = false, 0, 0, 0
	h.Warnings, h.FailuresDetail = nil, nil
}

func applyPerHostVerification(receipt *maintenance.Receipt, result *ansible.RunResult, runErr error) error {
	if runErr != nil {
		for i := range receipt.Hosts {
			receipt.Hosts[i].Verification = "unknown"
		}
		return runErr
	}
	if result == nil {
		return fmt.Errorf("verification returned no result")
	}
	byHost := map[string]ansible.HostResult{}
	for _, h := range result.Hosts {
		byHost[h.Host] = h
	}
	for i := range receipt.Hosts {
		h, ok := byHost[receipt.Hosts[i].Host]
		if !ok {
			receipt.Hosts[i].Verification = "unknown"
			continue
		}
		if h.Unreachable > 0 || h.Failures > 0 || h.Failed {
			receipt.Hosts[i].Verification = "failed"
			receipt.Hosts[i].Warnings = append(receipt.Hosts[i].Warnings, "postcondition failed on this host")
		} else {
			receipt.Hosts[i].Verification = "succeeded"
		}
	}
	for _, h := range receipt.Hosts {
		if h.Verification != "succeeded" {
			return fmt.Errorf("postcondition verification was not successful for host %s", h.Host)
		}
	}
	return nil
}

func revalidateBeforeBatch(ctx context.Context, cmdCtx *Context, cfg *config.Config, selected map[string]config.InventoryHost, plan maintenance.Plan, completed map[string]bool) error {
	current, err := captureMaintenanceSnapshot(ctx, cmdCtx, cfg, selected, plan)
	if err != nil {
		return err
	}
	expected := snapshotWithCompletedHosts(plan.Snapshot, current, completed)
	comparison := maintenance.CompareSnapshots(expected, current)
	if comparison.Overall != maintenance.DispositionMatch {
		return fmt.Errorf("plan/current safety comparison: changed=%v unknown=%v blockers=%v", comparison.ChangedFields, comparison.UnknownFields, comparison.Blockers)
	}
	return nil
}

func snapshotWithCompletedHosts(planned, current maintenance.SafetySnapshot, completed map[string]bool) maintenance.SafetySnapshot {
	expected := planned
	expected.Hosts = make(map[string]maintenance.HostSnapshot, len(planned.Hosts))
	for name, host := range planned.Hosts {
		expected.Hosts[name] = host
	}
	for name := range completed {
		if currentHost, ok := current.Hosts[name]; ok {
			expected.Hosts[name] = currentHost
		}
	}
	return expected
}

func captureMaintenanceSnapshot(ctx context.Context, cmdCtx *Context, cfg *config.Config, selected map[string]config.InventoryHost, plan maintenance.Plan) (maintenance.SafetySnapshot, error) {
	res, err := runCheckUpdates(ctx, hostSpecs(selected))
	if err != nil {
		return maintenance.SafetySnapshot{}, err
	}
	statuses := maintenance.InterpretCheckUpdates(res)
	byHost := map[string]maintenance.HostStatus{}
	for _, s := range statuses {
		byHost[s.Host] = s
	}
	snapshot := maintenance.SafetySnapshot{Version: 1, Hosts: map[string]maintenance.HostSnapshot{}, Infrastructure: plan.Snapshot.Infrastructure, Backup: plan.Snapshot.Backup, EvidenceComplete: res != nil && res.Success && res.ParseError == "" && !res.PartialFailure}
	for _, ph := range plan.Hosts {
		h, ok := selected[ph.Name]
		status, found := byHost[ph.Name]
		if !ok || !found {
			snapshot.EvidenceComplete = false
			snapshot.Hosts[ph.Name] = maintenance.HostSnapshot{Name: ph.Name, Address: ph.Address}
			continue
		}
		snapshot.Hosts[ph.Name] = maintenance.Snapshot(ph, status, h.SSHUser, h.SSHPort, h.SSHKeyFile != "", h.KnownHostsFile != "")
		if !status.EvidenceComplete {
			snapshot.EvidenceComplete = false
		}
	}
	if plan.Environment != "" {
		env, err := evaluateEnvironment(ctx, cmdCtx, cfg, plan.Environment, true)
		if err != nil {
			return maintenance.SafetySnapshot{}, err
		}
		snapshot.Infrastructure = maintenance.InfraSnapshot{Environment: env.Environment, Overall: string(env.Overall), MaintenanceSafe: env.MaintenanceSafe, Blockers: env.Blockers, Checks: environmentCheckStates(env), EvidenceComplete: !env.PartialFailure}
		snapshot.Backup = maintenanceBackupState(plan.Backup, env)
		snapshot.EvidenceComplete = snapshot.EvidenceComplete && snapshot.Infrastructure.EvidenceComplete
	}
	return snapshot, nil
}

func writeMaintenanceReport(cmdCtx *Context, r maintenanceReport) error {
	sort.Slice(r.Hosts, func(i, j int) bool { return r.Hosts[i].Host < r.Hosts[j].Host })
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, r)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, r)
	}
	rows := make([][]string, 0, len(r.Hosts))
	for _, h := range r.Hosts {
		rows = append(rows, []string{h.Host, h.Operation, h.State, boolYes(h.Success)})
	}
	if err := output.WriteTable(cmdCtx.Writer, []string{"HOST", "OPERATION", "STATE", "SUCCESS"}, rows); err != nil {
		return err
	}
	fmt.Fprintf(cmdCtx.Writer, "Plan: %s\nState: %s\nVerified: %t\nReceipt: %s\n", r.PlanID, r.State, r.Verified, r.ReceiptID)
	return nil
}

func runMaintenanceVerify(ctx context.Context, cmdCtx *Context, args []string) error {
	for _, arg := range args {
		if arg == "--receipt-dir" {
			return app.NewExitError(fmt.Errorf("usage: nodex maintenance verify --plan <file>"), app.ExitUsage)
		}
	}
	parsed, err := parseMaintenanceApplyArgs(args)
	if err != nil {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance verify --plan <file>: %w", err), app.ExitUsage)
	}
	planPath := parsed.planPath
	plan, err := maintenance.LoadFile(planPath, time.Now())
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected := map[string]config.InventoryHost{}
	for _, ph := range plan.Hosts {
		if cfg.Inventory == nil {
			return app.NewExitError(fmt.Errorf("inventory is not configured"), app.ExitConfig)
		}
		h, ok := cfg.Inventory.Hosts[ph.Name]
		if !ok || !inventoryHostMatchesPlan(ph, h) {
			return app.NewExitError(fmt.Errorf("inventory changed for planned host %q", ph.Name), app.ExitConflict)
		}
		selected[ph.Name] = h
	}
	result, runErr := runMaintenanceOperation(ctx, "verify-maintenance", hostSpecs(selected), nil)
	r := maintenanceReport{PlanID: plan.PlanID, PlanDigest: plan.Digest, State: "failed", Verified: false}
	if runErr == nil && result != nil && result.Success {
		r.State, r.Verified = "succeeded", true
	} else if runErr != nil {
		r.Error = runErr.Error()
	}
	for _, h := range plan.HostOrder {
		r.Hosts = append(r.Hosts, maintenance.HostReceipt{Host: h, Operation: "verify-maintenance", State: r.State, Success: r.Verified})
	}
	if err := writeMaintenanceReport(cmdCtx, r); err != nil {
		return err
	}
	if !r.Verified {
		return app.NewExitError(fmt.Errorf("postcondition verification failed"), app.ExitPartialFailure)
	}
	return nil
}

func runMaintenanceReport(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 || args[0] != "--receipt" {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance report --receipt <file>"), app.ExitUsage)
	}
	r, err := maintenance.LoadReceipt(args[1])
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	return writeMaintenanceReport(cmdCtx, maintenanceReport{PlanID: r.PlanID, PlanDigest: r.PlanDigest, ReceiptID: r.ReceiptID, State: r.State, Verified: r.State == "succeeded", Hosts: r.Hosts, Error: r.Error})
}

// runMaintenanceResume continues only hosts that have not reached a durable
// successful state in the receipt. It reuses apply's plan and safety checks.
func runMaintenanceResume(ctx context.Context, cmdCtx *Context, args []string) error {
	if !hasArg(args, "--receipt") {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance resume --plan <file> --receipt <file>"), app.ExitUsage)
	}
	return runMaintenanceApply(ctx, cmdCtx, args)
}

func runMaintenanceAbandon(_ context.Context, cmdCtx *Context, args []string) error {
	receiptPath, reason, err := receiptActionArgs(args)
	if err != nil || !cmdCtx.Opts.Yes || !cmdCtx.Opts.Force || cmdCtx.Opts.ConfirmTarget == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance abandon --receipt <file> --reason <reason> --yes --force --confirm-target <receipt-id>"), app.ExitUsage)
	}
	receipt, err := maintenance.LoadReceipt(receiptPath)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	if receipt.ReceiptID != cmdCtx.Opts.ConfirmTarget {
		return app.NewExitError(fmt.Errorf("confirmation refused: receipt ID does not match"), app.ExitConflict)
	}
	if receipt.State == "succeeded" || receipt.State == "abandoned" {
		return app.NewExitError(fmt.Errorf("receipt is already terminal: %s", receipt.State), app.ExitConflict)
	}
	receipt.State, receipt.Error = "abandoned", redact.String(reason)
	if err := persistReceipt(receiptPath, &receipt); err != nil {
		return err
	}
	return writeMaintenanceReport(cmdCtx, maintenanceReport{PlanID: receipt.PlanID, PlanDigest: receipt.PlanDigest, ReceiptID: receipt.ReceiptID, State: receipt.State, Hosts: receipt.Hosts, Error: receipt.Error})
}

func runMaintenanceReconcile(ctx context.Context, cmdCtx *Context, args []string) error {
	if !hasArg(args, "--receipt") {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance reconcile --plan <file> --receipt <file>"), app.ExitUsage)
	}
	parsed, err := parseMaintenanceApplyArgs(args)
	if err != nil || parsed.receiptPath == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance reconcile --plan <file> --receipt <file>"), app.ExitUsage)
	}
	plan, err := maintenance.LoadFile(parsed.planPath, time.Now())
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	receipt, err := maintenance.LoadReceipt(parsed.receiptPath)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	if err := receipt.VerifyForPlan(plan); err != nil {
		return app.NewExitError(err, app.ExitConflict)
	}
	return runMaintenanceVerify(ctx, cmdCtx, []string{"--plan", parsed.planPath})
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func receiptActionArgs(args []string) (path, reason string, err error) {
	for i := 0; i < len(args); i++ {
		if args[i] != "--receipt" && args[i] != "--reason" {
			return "", "", fmt.Errorf("unknown receipt argument %q", args[i])
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return "", "", fmt.Errorf("%s requires a value", args[i])
		}
		if args[i] == "--receipt" {
			path = args[i+1]
		} else {
			reason = args[i+1]
		}
		i++
	}
	if path == "" || reason == "" {
		return "", "", fmt.Errorf("receipt and reason are required")
	}
	return path, reason, nil
}
