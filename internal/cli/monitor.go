package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/backuphealth"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/monitor"
	"github.com/geoffmcc/nodex/internal/output"
)

func runMonitorTargets(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex monitor targets"), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	targets := map[string]config.MonitorTarget{}
	if cfg.Monitoring != nil {
		targets = cfg.Monitoring.Targets
	}
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		t := targets[name]
		rows = append(rows, []string{name, t.Type, monitor.SafeAddress(t.Address)})
	}
	safeTargets := make(map[string]config.MonitorTarget, len(targets))
	for name, target := range targets {
		target.Address = monitor.SafeAddress(target.Address)
		safeTargets[name] = target
	}
	if cmdCtx.Opts.Output == output.FormatJSON {
		return output.WriteJSON(cmdCtx.Writer, safeTargets)
	}
	if cmdCtx.Opts.Output == output.FormatYAML {
		return output.WriteYAML(cmdCtx.Writer, safeTargets)
	}
	return output.WriteTable(cmdCtx.Writer, []string{"NAME", "TYPE", "ADDRESS"}, rows)
}

func runMonitorCheck(ctx context.Context, cmdCtx *Context, args []string) error {
	targetName := ""
	environment := ""
	for i := 0; i < len(args); i++ {
		if args[i] != "--target" || i+1 >= len(args) {
			if args[i] == "--environment" && i+1 < len(args) {
				environment = args[i+1]
				i++
				continue
			}
			return app.NewExitError(fmt.Errorf("usage: nodex monitor check [--target <name>] [--environment <name>]"), app.ExitUsage)
		}
		targetName = args[i+1]
		i++
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	targets := map[string]config.MonitorTarget{}
	if cfg.Monitoring != nil {
		targets = cfg.Monitoring.Targets
	}
	if targetName != "" {
		t, ok := targets[targetName]
		if !ok {
			return app.NewExitError(fmt.Errorf("monitor target %q not found", targetName), app.ExitNotFound)
		}
		targets = map[string]config.MonitorTarget{targetName: t}
	}
	if environment != "" {
		filtered := make(map[string]config.MonitorTarget)
		for name, target := range targets {
			if target.Environment == environment {
				filtered[name] = target
			}
		}
		targets = filtered
	}
	if len(targets) == 0 {
		return app.NewExitError(fmt.Errorf("no monitoring targets are configured for the requested scope"), app.ExitConfig)
	}
	concurrency, globalTimeout := monitor.DefaultConcurrency, time.Duration(0)
	if cfg.Monitoring != nil {
		concurrency = cfg.Monitoring.Concurrency
		if cfg.Monitoring.Timeout > 0 {
			globalTimeout = time.Duration(cfg.Monitoring.Timeout) * time.Second
		}
	}
	providerCtx, cancelProvider := context.WithCancel(ctx)
	if globalTimeout > 0 {
		providerCtx, cancelProvider = context.WithTimeout(ctx, globalTimeout)
	}
	defer cancelProvider()
	providerResults := providerMonitorResults(providerCtx, cmdCtx, cfg, targets)
	report := monitor.CheckWithProviderOptions(ctx, targets, concurrency, globalTimeout, func(_ context.Context, name string, target config.MonitorTarget) (monitor.Result, bool) {
		result, ok := providerResults[name]
		return result, ok
	})
	if err := writeMonitorReport(cmdCtx, report); err != nil {
		return err
	}
	if report.Overall != monitor.Healthy {
		return app.NewExitError(fmt.Errorf("monitor check is %s", report.Overall), app.ExitPartialFailure)
	}
	return nil
}

func providerMonitorResults(ctx context.Context, cmdCtx *Context, cfg *config.Config, targets map[string]config.MonitorTarget) map[string]monitor.Result {
	results := make(map[string]monitor.Result)
	environmentResults := make(map[string]*backuphealth.Result)
	for name, target := range targets {
		if !providerMonitorType(target.Type) {
			continue
		}
		if target.Environment == "" {
			results[name] = monitor.Result{Name: name, Type: target.Type, State: monitor.Blocked, Detail: "provider-backed checks require an environment"}
			continue
		}
		env, ok := cfg.Environments[target.Environment]
		if !ok {
			results[name] = monitor.Result{Name: name, Type: target.Type, State: monitor.Blocked, Detail: "monitor environment is not configured"}
			continue
		}
		health, cached := environmentResults[target.Environment]
		if !cached {
			var err error
			health, err = evaluateEnvironment(ctx, cmdCtx, cfg, target.Environment, true)
			if err != nil {
				results[name] = monitor.Result{Name: name, Type: target.Type, State: monitor.Unknown, Detail: "provider-backed check could not be evaluated"}
				continue
			}
			environmentResults[target.Environment] = health
		}
		checkName := providerCheckName(target.Type)
		if target.Type == "backup-age" || target.Type == "backup-verification" || target.Type == "backup-coverage" {
			checkName = "guest_backup_coverage"
		}
		status, detail := backupHealthCheck(health, checkName)
		if target.Type == "pbs-tasks" {
			if activeStatus, activeDetail := backupHealthCheck(health, "pbs_active_tasks"); activeStatus != monitor.Healthy {
				status, detail = activeStatus, activeDetail
			} else if len(health.Blockers) > 0 {
				for _, blocker := range health.Blockers {
					if strings.Contains(blocker, "active PBS backup-chain task") {
						status, detail = monitor.Blocked, blocker
						break
					}
				}
			}
		}
		if target.Type == "pve-tasks" && env.PVEProfile == "" {
			status, detail = monitor.Unsupported, "no pve_profile configured"
		}
		if target.Type == "pbs-tasks" && env.PBSProfile == "" {
			status, detail = monitor.Unsupported, "no pbs_profile configured"
		}
		results[name] = monitor.Result{Name: name, Type: target.Type, State: status, Detail: detail}
	}
	return results
}

func providerMonitorType(kind string) bool {
	switch kind {
	case "pve-api", "pbs-api", "pve-tasks", "pbs-tasks", "datastore", "backup-age", "backup-verification", "backup-coverage":
		return true
	}
	return false
}

func providerCheckName(kind string) string {
	switch kind {
	case "pve-api", "pve-tasks":
		if kind == "pve-tasks" {
			return "pve_failed_backup_tasks"
		}
		return "pve_reachable"
	case "pbs-api", "pbs-tasks":
		if kind == "pbs-tasks" {
			return "pbs_failed_tasks"
		}
		return "pbs_reachable"
	case "datastore":
		return "pbs_datastores"
	}
	return "guest_backup_coverage"
}

func backupHealthCheck(result *backuphealth.Result, name string) (monitor.State, string) {
	if result == nil {
		return monitor.Unknown, "provider-backed check returned no result"
	}
	for _, check := range result.Checks {
		if check.Name == name {
			return monitorState(check.Status), check.Detail
		}
	}
	return monitor.Unknown, "provider-backed check was not reported"
}

func monitorState(status backuphealth.Status) monitor.State {
	switch status {
	case backuphealth.StatusHealthy:
		return monitor.Healthy
	case backuphealth.StatusWarning:
		return monitor.Degraded
	case backuphealth.StatusUnsupported:
		return monitor.Unsupported
	case backuphealth.StatusBlocked:
		return monitor.Blocked
	default:
		return monitor.Unknown
	}
}

func writeMonitorReport(cmdCtx *Context, report monitor.Report) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, report)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, report)
	default:
		rows := make([][]string, 0, len(report.Results))
		for _, r := range report.Results {
			rows = append(rows, []string{r.Name, string(r.State), r.Type, r.Detail, fmt.Sprintf("%d", r.Latency)})
		}
		return output.WriteTable(cmdCtx.Writer, []string{"NAME", "STATE", "TYPE", "DETAIL", "MS"}, rows)
	}
}
