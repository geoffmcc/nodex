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
		headers := []string{"NAME", "ADDRESS", "ROLE", "ENV", "GROUP", "CRITICALITY", "BACKUP-REQ", "AUTO-REBOOT"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{
				e.Name, e.Address, e.Role, e.Environment, e.MaintenanceGroup,
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
		PartialFailure: res.PartialFailure || res.ParseError != "",
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
			Name:           name,
			Address:        h.Address,
			Role:           h.Role,
			Group:          h.MaintenanceGroup,
			Criticality:    orDefault(h.Criticality, config.CriticalityStandard),
			BackupRequired: h.BackupRequired,
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
			}
		} else {
			plan.Blockers = append(plan.Blockers, fmt.Sprintf("host %s produced no preflight results", name))
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
			Environment:     envResult.Environment,
			Overall:         string(envResult.Overall),
			MaintenanceSafe: envResult.MaintenanceSafe,
			Blockers:        envResult.Blockers,
		}
		plan.Backup.Satisfied = envResult.MaintenanceSafe
		plan.Backup.Detail = fmt.Sprintf("environment %s backup health: %s", f.environment, envResult.Overall)
		if !envResult.MaintenanceSafe {
			plan.Blockers = append(plan.Blockers, envResult.Blockers...)
		}
	} else if len(plan.Backup.RequiredHosts) > 0 {
		plan.Backup.Satisfied = false
		plan.Backup.Detail = "backup state unknown: no --environment given for backup-required hosts"
		plan.Blockers = append(plan.Blockers, "backup requirements cannot be verified without --environment")
	} else {
		plan.Backup.Satisfied = true
		plan.Backup.Detail = "no hosts require backups"
	}

	if res.PartialFailure || res.ParseError != "" {
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

func parseMaintenanceApplyArgs(args []string) (string, string, error) {
	planPath, receiptDir := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plan", "--receipt-dir":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", fmt.Errorf("%s requires a value", args[i])
			}
			if args[i] == "--plan" {
				planPath = args[i+1]
			} else {
				receiptDir = args[i+1]
			}
			i++
		default:
			return "", "", fmt.Errorf("unknown maintenance apply argument %q", args[i])
		}
	}
	if planPath == "" {
		return "", "", fmt.Errorf("--plan is required")
	}
	if receiptDir == "" {
		receiptDir = filepath.Join(filepath.Dir(planPath), ".nodex-receipts")
	}
	return planPath, receiptDir, nil
}

func runMaintenanceApply(ctx context.Context, cmdCtx *Context, args []string) error {
	planPath, receiptDir, err := parseMaintenanceApplyArgs(args)
	if err != nil {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance apply --plan <file> [--receipt-dir <dir>]: %w", err), app.ExitUsage)
	}
	plan, err := maintenance.LoadFile(planPath, time.Now())
	if err != nil {
		return app.NewExitError(fmt.Errorf("load maintenance plan: %w", err), app.ExitValidationError)
	}
	if len(plan.Blockers) != 0 {
		return app.NewExitError(fmt.Errorf("plan %s has %d blocker(s); apply refused", plan.PlanID, len(plan.Blockers)), app.ExitValidationError)
	}
	if !cmdCtx.Opts.Yes || !cmdCtx.Opts.Force || cmdCtx.Opts.ConfirmTarget != plan.PlanID {
		return app.NewExitError(fmt.Errorf("confirmation refused: apply requires --yes --force --confirm-target %s", plan.PlanID), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	selected := map[string]config.InventoryHost{}
	for _, ph := range plan.Hosts {
		h, ok := cfg.Inventory.Hosts[ph.Name]
		if !ok || h.Address != ph.Address {
			return app.NewExitError(fmt.Errorf("inventory changed for planned host %q; create a new plan", ph.Name), app.ExitConflict)
		}
		selected[ph.Name] = h
	}
	receipt := maintenance.NewReceipt(plan, time.Now())
	if err := receipt.Finalize(); err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	path := maintenance.ReceiptPath(receiptDir, plan.PlanID)
	if _, statErr := os.Stat(path); statErr == nil {
		if _, loadErr := maintenance.LoadReceipt(path); loadErr != nil {
			return app.NewExitError(fmt.Errorf("existing receipt is invalid; refusing to overwrite: %w", loadErr), app.ExitValidationError)
		}
		return app.NewExitError(fmt.Errorf("receipt already exists at %s; refusing to rerun without operator review", path), app.ExitConflict)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("inspect receipt: %w", statErr)
	}
	if err := maintenance.SaveReceipt(path, receipt); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	op := "apply-security-updates"
	if plan.Policy == maintenance.PolicyApprovedFull {
		op = "apply-approved-updates"
	}
	for _, name := range plan.HostOrder {
		packages := []string{}
		if op == "apply-security-updates" {
			for _, planned := range plan.Hosts {
				if planned.Name == name {
					packages = planned.SecurityUpdates
					break
				}
			}
		}
		result, runErr := runMaintenanceOperation(ctx, op, hostSpecs(map[string]config.InventoryHost{name: selected[name]}), packages)
		h := maintenance.HostReceipt{Host: name, Operation: op, State: "failed"}
		if runErr != nil {
			h.State = "unknown"
			receipt.State = "unknown"
			receipt.Error = runErr.Error()
		} else if result == nil || !result.Success {
			h.State = "failed"
			receipt.State = "failed"
			receipt.Error = "Ansible reported an unsuccessful or unverifiable result"
			if result != nil && len(result.Hosts) == 1 {
				h.Changed, h.Failures, h.Unreachable = result.Hosts[0].Changed, result.Hosts[0].Failures, result.Hosts[0].Unreachable
			}
		} else {
			h.State, h.Success = "succeeded", true
			if len(result.Hosts) == 1 {
				h.Changed = result.Hosts[0].Changed
			}
		}
		receipt.Hosts = append(receipt.Hosts, h)
		receipt.UpdatedAt = time.Now().Unix()
		if receipt.State == "running" && h.State != "succeeded" {
			receipt.State = h.State
		}
		if err := receipt.Finalize(); err != nil {
			return err
		}
		if err := maintenance.SaveReceipt(path, receipt); err != nil {
			return fmt.Errorf("checkpoint receipt: %w", err)
		}
		if h.State != "succeeded" {
			return app.NewExitError(fmt.Errorf("maintenance apply stopped at host %s; receipt: %s", name, path), app.ExitAmbiguousOutcome)
		}
	}
	verifyHosts := hostSpecs(selected)
	verification, runErr := runMaintenanceOperation(ctx, "verify-maintenance", verifyHosts, nil)
	if runErr != nil || verification == nil || !verification.Success {
		receipt.State = "failed"
		receipt.Error = "postcondition verification failed or was unverifiable"
		if runErr != nil {
			receipt.Error = runErr.Error()
		}
		receipt.UpdatedAt = time.Now().Unix()
		_ = receipt.Finalize()
		_ = maintenance.SaveReceipt(path, receipt)
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
	planPath, _, err := parseMaintenanceApplyArgs(args)
	if err != nil {
		return app.NewExitError(fmt.Errorf("usage: nodex maintenance verify --plan <file>: %w", err), app.ExitUsage)
	}
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
		h, ok := cfg.Inventory.Hosts[ph.Name]
		if !ok || h.Address != ph.Address {
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
