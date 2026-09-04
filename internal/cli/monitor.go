package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/geoffmcc/nodex/internal/app"
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
		rows = append(rows, []string{name, t.Type, t.Address})
	}
	if cmdCtx.Opts.Output == output.FormatJSON {
		return output.WriteJSON(cmdCtx.Writer, targets)
	}
	if cmdCtx.Opts.Output == output.FormatYAML {
		return output.WriteYAML(cmdCtx.Writer, targets)
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
	report := monitor.Check(ctx, targets)
	if err := writeMonitorReport(cmdCtx, report); err != nil {
		return err
	}
	for _, result := range report.Results {
		if result.State != monitor.Healthy {
			return app.NewExitError(fmt.Errorf("monitor check is %s", result.State), app.ExitPartialFailure)
		}
	}
	return nil
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
