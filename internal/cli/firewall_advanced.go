package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func runFirewallAliases(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall aliases"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	aliases, err := fw.FirewallAliases(ctx)
	if err != nil {
		return fmt.Errorf("list firewall aliases: %w", err)
	}
	return writeFirewallAliases(cmdCtx, aliases)
}

func writeFirewallAliases(cmdCtx *Context, aliases []domain.FirewallAlias) error {
	if aliases == nil {
		aliases = []domain.FirewallAlias{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, aliases)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, aliases)
	default:
		headers := []string{"NAME", "CIDR", "COMMENT"}
		rows := make([][]string, 0, len(aliases))
		for _, a := range aliases {
			rows = append(rows, []string{a.Name, a.CIDR, a.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runFirewallIPSets(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipsets"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	ipsets, err := fw.FirewallIPSets(ctx)
	if err != nil {
		return fmt.Errorf("list firewall IP sets: %w", err)
	}
	return writeFirewallIPSets(cmdCtx, ipsets)
}

func writeFirewallIPSets(cmdCtx *Context, ipsets []domain.FirewallIPSet) error {
	if ipsets == nil {
		ipsets = []domain.FirewallIPSet{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, ipsets)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, ipsets)
	default:
		headers := []string{"NAME", "COMMENT"}
		rows := make([][]string, 0, len(ipsets))
		for _, s := range ipsets {
			rows = append(rows, []string{s.Name, s.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runFirewallIPSet(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset <name>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	entries, err := fw.FirewallIPSet(ctx, args[0])
	if err != nil {
		return fmt.Errorf("get firewall IP set: %w", err)
	}
	return writeFirewallIPSetEntries(cmdCtx, entries)
}

func writeFirewallIPSetEntries(cmdCtx *Context, entries []domain.FirewallIPSetEntry) error {
	if entries == nil {
		entries = []domain.FirewallIPSetEntry{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, entries)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, entries)
	default:
		headers := []string{"CIDR", "COMMENT"}
		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{e.CIDR, e.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runFirewallSecurityGroups(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall security-groups"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	groups, err := fw.FirewallSecurityGroups(ctx)
	if err != nil {
		return fmt.Errorf("list firewall security groups: %w", err)
	}
	return writeFirewallSecurityGroups(cmdCtx, groups)
}

func writeFirewallSecurityGroups(cmdCtx *Context, groups []domain.FirewallSecurityGroup) error {
	if groups == nil {
		groups = []domain.FirewallSecurityGroup{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, groups)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, groups)
	default:
		headers := []string{"NAME", "RULES", "COMMENT"}
		rows := make([][]string, 0, len(groups))
		for _, g := range groups {
			rows = append(rows, []string{g.Name, fmt.Sprintf("%d", len(g.Rules)), g.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runFirewallOptions(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall options"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	opts, err := fw.FirewallOptions(ctx)
	if err != nil {
		return fmt.Errorf("get firewall options: %w", err)
	}
	return writeFirewallOptionsTable(cmdCtx, opts)
}

func writeFirewallOptionsTable(cmdCtx *Context, opts *domain.FirewallOptions) error {
	if opts == nil {
		opts = &domain.FirewallOptions{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, opts)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, opts)
	default:
		rows := [][]string{
			{"ENABLE", fmt.Sprintf("%d", opts.Enable)},
			{"LOG IN DROP", fmt.Sprintf("%d", opts.Log)},
		}
		return output.WriteTable(cmdCtx.Writer, []string{"FIELD", "VALUE"}, rows)
	}
}

func runFirewallNodeRules(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall node-rules <node>"), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	rules, err := fw.NodeFirewallRules(ctx, args[0])
	if err != nil {
		return fmt.Errorf("get node firewall rules: %w", err)
	}
	return writeFirewallRules(cmdCtx, rules)
}

func runFirewallVMRules(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall vm-rules <node>/<vmid>"), app.ExitUsage)
	}
	parts := strings.SplitN(args[0], "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall vm-rules <node>/<vmid>"), app.ExitUsage)
	}
	node := parts[0]
	vmid, err := strconv.Atoi(parts[1])
	if err != nil || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("invalid VMID: %s", parts[1]), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fw, err := requireFirewallAdvanced(prov)
	if err != nil {
		return err
	}
	rules, err := fw.VMFirewallRules(ctx, node, vmid)
	if err != nil {
		return fmt.Errorf("get VM firewall rules: %w", err)
	}
	return writeFirewallRules(cmdCtx, rules)
}
