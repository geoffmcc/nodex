package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/backuphealth"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// Handlers for the `nodex environment` command group: unified PVE/PBS
// environment health and backup health. Overall status maps to exit codes:
// healthy and warning exit 0; unsupported, unknown, blocked, or partial
// failure exit with ExitPartial so schedulers can alert.

// downProvider stands in for a configured provider whose connection failed:
// reachability checks surface the stored error instead of "unsupported".
type downProvider struct {
	name string
	err  error
}

func (d *downProvider) Name() string    { return d.name }
func (d *downProvider) Version() string { return "" }
func (d *downProvider) Connect(context.Context, string, *domain.Credentials) error {
	return d.err
}
func (d *downProvider) Close() error                      { return nil }
func (d *downProvider) Health(context.Context) error      { return d.err }
func (d *downProvider) Capabilities() []domain.Capability { return nil }

func runEnvironmentList(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex environment list"), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	names := config.EnvironmentNames(cfg)

	type envEntry struct {
		Name       string `json:"name" yaml:"name"`
		PVEProfile string `json:"pve_profile,omitempty" yaml:"pve_profile,omitempty"`
		PBSProfile string `json:"pbs_profile,omitempty" yaml:"pbs_profile,omitempty"`
	}
	entries := make([]envEntry, 0, len(names))
	for _, name := range names {
		env := cfg.Environments[name]
		entries = append(entries, envEntry{Name: name, PVEProfile: env.PVEProfile, PBSProfile: env.PBSProfile})
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, entries)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, entries)
	default:
		if len(entries) == 0 {
			fmt.Fprintln(cmdCtx.Writer, "No environments configured. Add an \"environments\" section to the configuration (schema version 2).")
			return nil
		}
		headers := []string{"NAME", "PVE-PROFILE", "PBS-PROFILE"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{e.Name, e.PVEProfile, e.PBSProfile})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runEnvironmentHealth(ctx context.Context, cmdCtx *Context, args []string) error {
	return runEnvironmentCheck(ctx, cmdCtx, args, false, "usage: nodex environment health <name>")
}

func runEnvironmentBackupHealth(ctx context.Context, cmdCtx *Context, args []string) error {
	return runEnvironmentCheck(ctx, cmdCtx, args, true, "usage: nodex environment backup-health <name>")
}

// evaluateEnvironment loads the named environment, connects its profiles
// (connection failures become reachability findings, not command failures),
// and returns the backup-health result. Callers own output and exit codes.
func evaluateEnvironment(ctx context.Context, cmdCtx *Context, cfg *config.Config, name string, includeGuests bool) (*backuphealth.Result, error) {
	env, ok := cfg.Environments[name]
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("environment %q not found in configuration", name),
			app.ExitConfig,
		)
	}

	var pve, pbs domain.Provider
	var cleanups []func()
	defer func() {
		for _, c := range cleanups {
			c()
		}
	}()
	if env.PVEProfile != "" {
		prov, cleanup, err := connectProfile(ctx, cmdCtx, env.PVEProfile)
		if err != nil {
			pve = &downProvider{name: config.ProviderProxmox, err: err}
		} else {
			pve = prov
			cleanups = append(cleanups, cleanup)
		}
	}
	if env.PBSProfile != "" {
		prov, cleanup, err := connectProfile(ctx, cmdCtx, env.PBSProfile)
		if err != nil {
			pbs = &downProvider{name: config.ProviderPBS, err: err}
		} else {
			pbs = prov
			cleanups = append(cleanups, cleanup)
		}
	}

	svc := &backuphealth.Service{PVE: pve, PBS: pbs}
	req := backuphealth.Request{
		Environment:   name,
		Thresholds:    environmentThresholds(env),
		Namespaces:    env.Namespaces,
		ExcludeGuests: env.ExcludeGuests,
		IncludeGuests: includeGuests,
	}
	result, err := svc.CheckEnvironmentBackupHealth(ctx, req)
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("evaluate environment %q: %w", name, err), app.ExitValidationError)
	}
	return result, nil
}

func runEnvironmentCheck(ctx context.Context, cmdCtx *Context, args []string, includeGuests bool, usage string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("%s", usage), app.ExitUsage)
	}
	name := args[0]
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	result, err := evaluateEnvironment(ctx, cmdCtx, cfg, name, includeGuests)
	if err != nil {
		return err
	}

	if err := writeEnvironmentResult(cmdCtx, result); err != nil {
		return err
	}
	switch result.Overall {
	case backuphealth.StatusHealthy, backuphealth.StatusWarning:
		if result.PartialFailure {
			return app.NewExitError(
				fmt.Errorf("environment %q evaluation incomplete: required data could not be retrieved", name),
				app.ExitPartialFailure,
			)
		}
		return nil
	default:
		return app.NewExitError(
			fmt.Errorf("environment %q is %s", name, result.Overall),
			app.ExitPartialFailure,
		)
	}
}

// environmentThresholds applies defaults to unset environment thresholds.
func environmentThresholds(env config.Environment) backuphealth.Thresholds {
	backupHours := env.BackupMaxAgeHours
	if backupHours == 0 {
		backupHours = config.DefaultBackupMaxAgeHours
	}
	verifyDays := env.VerifyMaxAgeDays
	if verifyDays == 0 {
		verifyDays = config.DefaultVerifyMaxAgeDays
	}
	warn := env.DatastoreWarnPercent
	if warn == 0 {
		warn = config.DefaultDatastoreWarnPercent
	}
	block := env.DatastoreBlockPercent
	if block == 0 {
		block = config.DefaultDatastoreBlockPercent
	}
	return backuphealth.Thresholds{
		BackupMaxAge:          time.Duration(backupHours) * time.Hour,
		VerifyMaxAge:          time.Duration(verifyDays) * 24 * time.Hour,
		DatastoreWarnPercent:  warn,
		DatastoreBlockPercent: block,
	}
}

func writeEnvironmentResult(cmdCtx *Context, result *backuphealth.Result) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, result)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, result)
	default:
		fmt.Fprintf(cmdCtx.Writer, "Environment:      %s\n", result.Environment)
		fmt.Fprintf(cmdCtx.Writer, "Overall:          %s\n", result.Overall)
		fmt.Fprintf(cmdCtx.Writer, "Maintenance safe: %t\n", result.MaintenanceSafe)
		if result.PartialFailure {
			fmt.Fprintln(cmdCtx.Writer, "Partial failure:  some data could not be retrieved")
		}
		fmt.Fprintln(cmdCtx.Writer)

		headers := []string{"CHECK", "STATUS", "DETAIL"}
		rows := make([][]string, 0, len(result.Checks))
		for _, c := range result.Checks {
			rows = append(rows, []string{c.Name, string(c.Status), c.Detail})
		}
		if err := output.WriteTable(cmdCtx.Writer, headers, rows); err != nil {
			return err
		}

		if len(result.Guests) > 0 {
			fmt.Fprintln(cmdCtx.Writer)
			gHeaders := []string{"VMID", "NAME", "TYPE", "STATUS", "AGE-H", "DATASTORE", "NAMESPACE", "VERIFIED", "DETAIL"}
			gRows := make([][]string, 0, len(result.Guests))
			for _, g := range result.Guests {
				age := ""
				if g.NewestBackup > 0 {
					age = strconv.FormatInt(g.AgeHours, 10)
				}
				gRows = append(gRows, []string{
					strconv.Itoa(g.VMID), g.Name, g.Type, string(g.Status),
					age, g.Datastore, g.Namespace, g.Verification, g.Detail,
				})
			}
			if err := output.WriteTable(cmdCtx.Writer, gHeaders, gRows); err != nil {
				return err
			}
		}

		if len(result.Blockers) > 0 {
			fmt.Fprintln(cmdCtx.Writer)
			fmt.Fprintln(cmdCtx.Writer, "Maintenance blockers:")
			for _, b := range result.Blockers {
				fmt.Fprintf(cmdCtx.Writer, "  - %s\n", b)
			}
		}
		return nil
	}
}
