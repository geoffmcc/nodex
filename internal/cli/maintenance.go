package cli

import (
	"context"
	"fmt"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runBackupList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex backup list <node>"), app.ExitUsage)
	}
	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex backup list <node>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	backups, err := prov.Backups(ctx, node)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	return writeBackups(cmdCtx, applyLimit(backups, cmdCtx.Opts.Limit))
}

func writeBackups(cmdCtx *Context, backups []domain.Backup) error {
	if backups == nil {
		backups = []domain.Backup{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, backups)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, backups)
	default:
		headers := []string{"UPID", "STATE", "STATUS", "STARTED", "NODE"}
		rows := make([][]string, 0, len(backups))
		for _, b := range backups {
			rows = append(rows, []string{
				b.UPID,
				b.State,
				b.Status,
				fmt.Sprintf("%d", b.StartTime),
				b.Node,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runFirewallList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	rules, err := prov.FirewallRules(ctx)
	if err != nil {
		return fmt.Errorf("list firewall rules: %w", err)
	}
	return writeFirewallRules(cmdCtx, applyLimit(rules, cmdCtx.Opts.Limit))
}

func writeFirewallRules(cmdCtx *Context, rules []domain.FirewallRule) error {
	if rules == nil {
		rules = []domain.FirewallRule{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, rules)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, rules)
	default:
		headers := []string{"POS", "TYPE", "ACTION", "PROTO", "SOURCE", "DEST", "COMMENT"}
		rows := make([][]string, 0, len(rules))
		for _, r := range rules {
			source := r.Source
			if r.Sport != "" {
				source += ":" + r.Sport
			}
			dest := r.Dest
			if r.Dport != "" {
				dest += ":" + r.Dport
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", r.Pos),
				r.Type,
				r.Action,
				r.Proto,
				source,
				dest,
				r.Comment,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runHAList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha list"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	resources, err := prov.HAResources(ctx)
	if err != nil {
		return fmt.Errorf("list HA resources: %w", err)
	}
	return writeHAResources(cmdCtx, applyLimit(resources, cmdCtx.Opts.Limit))
}

func writeHAResources(cmdCtx *Context, resources []domain.HAResource) error {
	if resources == nil {
		resources = []domain.HAResource{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, resources)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, resources)
	default:
		headers := []string{"ID", "TYPE", "STATE", "NODE", "GROUP"}
		rows := make([][]string, 0, len(resources))
		for _, r := range resources {
			rows = append(rows, []string{
				r.ID,
				r.Type,
				r.State,
				r.Node,
				r.Group,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runHAGroups(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha groups"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	groups, err := prov.HAGroups(ctx)
	if err != nil {
		return fmt.Errorf("list HA groups: %w", err)
	}
	return writeHAGroups(cmdCtx, applyLimit(groups, cmdCtx.Opts.Limit))
}

func writeHAGroups(cmdCtx *Context, groups []domain.HAGroup) error {
	if groups == nil {
		groups = []domain.HAGroup{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, groups)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, groups)
	default:
		headers := []string{"ID", "TYPE", "NODES", "COMMENT"}
		rows := make([][]string, 0, len(groups))
		for _, g := range groups {
			rows = append(rows, []string{
				g.ID,
				g.Type,
				g.Nodes,
				g.Comment,
			})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}
