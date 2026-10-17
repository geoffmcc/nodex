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

	bi, err := requireBackupInspector(prov)
	if err != nil {
		return err
	}
	backups, err := bi.Backups(ctx, node)
	if err != nil {
		return err
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

// runFirewallClusterRules lists cluster-wide firewall rules from
// /cluster/firewall/rules. Node- and VM-scoped rules have their own commands so
// the three scopes are always distinguishable by name (nit #34).
func runFirewallClusterRules(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall <cluster-rules|list|rules>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fi, err := requireFirewallInspector(prov)
	if err != nil {
		return err
	}
	rules, err := fi.FirewallRules(ctx)
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

	ha, err := requireHAInspector(prov)
	if err != nil {
		return err
	}
	resources, err := ha.HAResources(ctx)
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

	ha, err := requireHAInspector(prov)
	if err != nil {
		return err
	}
	groups, err := ha.HAGroups(ctx)
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

func runHAResourceDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha resource <add|state|remove> ..."), app.ExitUsage)
	}
	switch args[0] {
	case "add":
		return runHAResourceAdd(ctx, cmdCtx, args[1:])
	case "state":
		return runHAResourceState(ctx, cmdCtx, args[1:])
	case "remove":
		return runHAResourceRemove(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(fmt.Errorf("usage: nodex ha resource <add|state|remove> ..."), app.ExitUsage)
	}
}

func runHAResourceAdd(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha resource add <vm:id|ct:id> [started|stopped|disabled]"), app.ExitUsage)
	}
	id := args[0]
	state := "started"
	if len(args) == 2 {
		state = args[1]
	}
	if err := checkDestructive(cmdCtx, "add guest to HA management", id); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	mutator, err := requireHAResourceMutator(prov)
	if err != nil {
		return err
	}
	if err := mutator.HAResourceAdd(ctx, id, state); err != nil {
		return fmt.Errorf("add HA resource %q: %w", id, err)
	}
	fmt.Fprintf(cmdCtx.Writer, "Added %s to HA management (state: %s)\n", id, state)
	return nil
}

func runHAResourceState(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha resource state <vm:id|ct:id> <started|stopped|disabled>"), app.ExitUsage)
	}
	id, state := args[0], args[1]
	if err := checkDestructive(cmdCtx, "change HA resource state", id); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	mutator, err := requireHAResourceMutator(prov)
	if err != nil {
		return err
	}
	if err := mutator.HAResourceSetState(ctx, id, state); err != nil {
		return fmt.Errorf("set HA resource %q state: %w", id, err)
	}
	fmt.Fprintf(cmdCtx.Writer, "Set HA resource %s state to %s\n", id, state)
	return nil
}

func runHAResourceRemove(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex ha resource remove <vm:id|ct:id>"), app.ExitUsage)
	}
	id := args[0]
	if err := checkDestructive(cmdCtx, "remove guest from HA management", id); err != nil {
		return err
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()
	mutator, err := requireHAResourceMutator(prov)
	if err != nil {
		return err
	}
	if err := mutator.HAResourceRemove(ctx, id); err != nil {
		return fmt.Errorf("remove HA resource %q: %w", id, err)
	}
	fmt.Fprintf(cmdCtx.Writer, "Removed %s from HA management\n", id)
	return nil
}

func requireHAResourceMutator(prov domain.Provider) (domain.HAResourceMutator, error) {
	mutator, ok := prov.(domain.HAResourceMutator)
	if !ok {
		return nil, app.NewExitError(fmt.Errorf("%w: HA resource management is not supported by provider %q", app.ErrUnsupportedCap, prov.Name()), app.ExitUnsupportedCap)
	}
	return mutator, nil
}
