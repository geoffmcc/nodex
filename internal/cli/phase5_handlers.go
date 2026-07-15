package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/safety"
)

// --- Network mutation handlers ---

// nodex network show <node> (read-only, already existing as node network)
// nodex network apply <node> <config-file> (Tier 2 with lockout warning)
// nodex network revert <node> (Tier 2)

// runNetworkApply applies network configuration from a file.
func runNetworkApply(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex network apply <node> <config-file>"), app.ExitUsage)
	}

	node := args[0]
	configFile := args[1]
	if node == "" || configFile == "" {
		return app.NewExitError(fmt.Errorf("node and config-file are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	nm, err := requireNetworkMutation(prov)
	if err != nil {
		return err
	}

	// Read config file
	data, err := os.ReadFile(configFile) // #nosec G304 -- configFile is user-specified config path, validated by caller.
	if err != nil {
		return fmt.Errorf("read config file %s: %w", configFile, err)
	}

	// Parse as key=value pairs (simple format)
	config := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid config line: %s (expected key=value)", line), app.ExitUsage)
		}
		config[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	if len(config) == 0 {
		return app.NewExitError(fmt.Errorf("config file %s is empty", configFile), app.ExitUsage)
	}

	// Tier 2 safety with lockout warning
	desc := fmt.Sprintf("network configuration on node %s", node)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: Incorrect network configuration can cause cluster lockout.\n")
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: Verify the configuration carefully before applying.\n")
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
	}

	if err := nm.ApplyNodeNetwork(ctx, node, config); err != nil {
		return fmt.Errorf("apply network config on %s: %w", node, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Network configuration applied on %s\n", node)
	return nil
}

// runNetworkRevert reverts pending network changes on a node.
func runNetworkRevert(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex network revert <node>"), app.ExitUsage)
	}

	node := args[0]
	if node == "" {
		return app.NewExitError(fmt.Errorf("node name is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	nm, err := requireNetworkMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("revert network configuration on node %s", node)
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierDisruptive,
		ResourceDescription: desc,
	}
	result := policy.Check(cmdCtx.Opts.Yes, cmdCtx.Opts.Force, cmdCtx.Opts.NonInteractive)
	if result.ConfirmationRequired {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(fmt.Errorf("confirmation required: %s", result.Message), app.ExitUsage)
		}
		fmt.Fprintf(cmdCtx.ErrW, "WARNING: Reverting network changes may disrupt current connectivity.\n")
		if result.Warning != "" {
			fmt.Fprintf(cmdCtx.ErrW, "WARNING: %s\n", result.Warning)
		}
		fmt.Fprintf(cmdCtx.ErrW, "%s\n", result.Message)
		return fmt.Errorf("%w: %s", safety.ErrAuthorizationRequired, result.Message)
	}

	if err := nm.RevertNodeNetwork(ctx, node); err != nil {
		return fmt.Errorf("revert network config on %s: %w", node, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Network configuration reverted on %s\n", node)
	return nil
}

// --- Firewall rule CRUD handlers ---

// runFirewallRuleDispatch dispatches firewall rule subcommands.
func runFirewallRuleDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex firewall rule <create|update|delete> <scope> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  create cluster --action <a> --type <t> [options]")
		fmt.Fprintln(cmdCtx.Writer, "  create node <node> --action <a> --type <t> [options]")
		fmt.Fprintln(cmdCtx.Writer, "  create vm <node>/<vmid> --action <a> --type <t> [options]")
		fmt.Fprintln(cmdCtx.Writer, "  update cluster <pos> [key=value ...]")
		fmt.Fprintln(cmdCtx.Writer, "  update node <node> <pos> [key=value ...]")
		fmt.Fprintln(cmdCtx.Writer, "  update vm <node>/<vmid> <pos> [key=value ...]")
		fmt.Fprintln(cmdCtx.Writer, "  delete cluster <pos>")
		fmt.Fprintln(cmdCtx.Writer, "  delete node <node> <pos>")
		fmt.Fprintln(cmdCtx.Writer, "  delete vm <node>/<vmid> <pos>")
		return nil
	}

	switch args[0] {
	case "create":
		return runFirewallRuleCreate(ctx, cmdCtx, args[1:])
	case "update":
		return runFirewallRuleUpdate(ctx, cmdCtx, args[1:])
	case "delete":
		return runFirewallRuleDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown firewall rule subcommand: %s (use create, update, or delete)", args[0]),
			app.ExitUsage,
		)
	}
}

// parseFirewallRuleArgs parses key=value arguments into a FirewallRuleCreateInput.
func parseFirewallRuleArgs(args []string) (domain.FirewallRuleCreateInput, error) {
	var input domain.FirewallRuleCreateInput
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return input, fmt.Errorf("invalid parameter: %s (expected key=value)", arg)
		}
		key := strings.TrimPrefix(parts[0], "--")
		val := parts[1]
		switch key {
		case "action":
			input.Action = val
		case "type":
			input.Type = val
		case "proto":
			input.Proto = val
		case "dest":
			input.Dest = val
		case "dport":
			input.Dport = val
		case "source":
			input.Source = val
		case "sport":
			input.Sport = val
		case "icmp_type", "icmp-type":
			input.ICMPType = val
		case "log":
			input.Log = val
		case "comment":
			input.Comment = val
		case "iface":
			input.IFace = val
		case "macro":
			input.Macro = val
		case "enable":
			if val == "1" || val == "true" {
				input.Enable = 1
			}
		case "pos":
			v, err := strconv.Atoi(val)
			if err != nil {
				return input, fmt.Errorf("invalid position: %s", val)
			}
			input.Pos = v
		default:
			return input, fmt.Errorf("unknown rule parameter: %s", key)
		}
	}
	return input, nil
}

// runFirewallRuleCreate creates a firewall rule.
func runFirewallRuleCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex firewall rule create cluster --action <accept|deny|reject> --type <in|out|group> [options]\n"+
				"       nodex firewall rule create node <node> --action <a> --type <t> [options]\n"+
				"       nodex firewall rule create vm <node>/<vmid> --action <a> --type <t> [options]"),
			app.ExitUsage)
	}

	scope := args[0]
	var node string
	var vmid int
	var remaining []string

	switch scope {
	case "cluster":
		remaining = args[1:]
	case "node":
		if len(args) < 2 {
			return app.NewExitError(fmt.Errorf("node name is required for node scope"), app.ExitUsage)
		}
		node = args[1]
		remaining = args[2:]
	case "vm":
		if len(args) < 2 {
			return app.NewExitError(fmt.Errorf("VM target <node>/<vmid> is required for VM scope"), app.ExitUsage)
		}
		var err error
		node, vmid, err = parseNodeVMID(args[1])
		if err != nil {
			return app.NewExitError(err, app.ExitUsage)
		}
		remaining = args[2:]
	default:
		return app.NewExitError(fmt.Errorf("unknown scope: %s (use cluster, node, or vm)", scope), app.ExitUsage)
	}

	input, err := parseFirewallRuleArgs(remaining)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if input.Type == "" {
		return app.NewExitError(fmt.Errorf("--type is required (in, out, or group)"), app.ExitUsage)
	}
	if input.Action == "" {
		return app.NewExitError(fmt.Errorf("--action is required (accept, deny, reject)"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	var desc string
	switch scope {
	case "cluster":
		desc = fmt.Sprintf("firewall rule (cluster, action: %s, type: %s)", input.Action, input.Type)
	case "node":
		desc = fmt.Sprintf("firewall rule (node %s, action: %s, type: %s)", node, input.Action, input.Type)
	case "vm":
		desc = fmt.Sprintf("firewall rule (VM %s/%d, action: %s, type: %s)", node, vmid, input.Action, input.Type)
	}

	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	var rule *domain.FirewallRule
	switch scope {
	case "cluster":
		rule, err = fm.CreateFirewallRule(ctx, input)
	case "node":
		rule, err = fm.CreateNodeFirewallRule(ctx, node, input)
	case "vm":
		rule, err = fm.CreateVMFirewallRule(ctx, node, vmid, input)
	}
	if err != nil {
		return fmt.Errorf("create firewall rule: %w", err)
	}

	return writeFirewallRules(cmdCtx, []domain.FirewallRule{*rule})
}

// runFirewallRuleUpdate updates a firewall rule.
func runFirewallRuleUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex firewall rule update cluster <pos> [key=value ...]\n"+
				"       nodex firewall rule update node <node> <pos> [key=value ...]\n"+
				"       nodex firewall rule update vm <node>/<vmid> <pos> [key=value ...]"),
			app.ExitUsage)
	}

	scope := args[0]
	var node string
	var vmid int
	var pos int
	var remaining []string

	switch scope {
	case "cluster":
		p, err := strconv.Atoi(args[1])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[1]), app.ExitUsage)
		}
		pos = p
		remaining = args[2:]
	case "node":
		if len(args) < 3 {
			return app.NewExitError(fmt.Errorf("node name and position are required for node scope"), app.ExitUsage)
		}
		node = args[1]
		p, err := strconv.Atoi(args[2])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[2]), app.ExitUsage)
		}
		pos = p
		remaining = args[3:]
	case "vm":
		if len(args) < 3 {
			return app.NewExitError(fmt.Errorf("VM target and position are required for VM scope"), app.ExitUsage)
		}
		var err error
		node, vmid, err = parseNodeVMID(args[1])
		if err != nil {
			return app.NewExitError(err, app.ExitUsage)
		}
		p, err := strconv.Atoi(args[2])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[2]), app.ExitUsage)
		}
		pos = p
		remaining = args[3:]
	default:
		return app.NewExitError(fmt.Errorf("unknown scope: %s (use cluster, node, or vm)", scope), app.ExitUsage)
	}

	input, err := parseFirewallRuleArgs(remaining)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	var desc string
	switch scope {
	case "cluster":
		desc = fmt.Sprintf("firewall rule pos %d (cluster)", pos)
	case "node":
		desc = fmt.Sprintf("firewall rule pos %d (node %s)", pos, node)
	case "vm":
		desc = fmt.Sprintf("firewall rule pos %d (VM %s/%d)", pos, node, vmid)
	}

	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	switch scope {
	case "cluster":
		err = fm.UpdateFirewallRule(ctx, pos, input)
	case "node":
		err = fm.UpdateNodeFirewallRule(ctx, node, pos, input)
	case "vm":
		err = fm.UpdateVMFirewallRule(ctx, node, vmid, pos, input)
	}
	if err != nil {
		return fmt.Errorf("update firewall rule: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall rule at position %d updated\n", pos)
	return nil
}

// runFirewallRuleDelete deletes a firewall rule.
func runFirewallRuleDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex firewall rule delete cluster <pos>\n"+
				"       nodex firewall rule delete node <node> <pos>\n"+
				"       nodex firewall rule delete vm <node>/<vmid> <pos>"),
			app.ExitUsage)
	}

	scope := args[0]
	var node string
	var vmid int
	var pos int

	switch scope {
	case "cluster":
		p, err := strconv.Atoi(args[1])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[1]), app.ExitUsage)
		}
		pos = p
	case "node":
		if len(args) < 3 {
			return app.NewExitError(fmt.Errorf("node name and position are required for node scope"), app.ExitUsage)
		}
		node = args[1]
		p, err := strconv.Atoi(args[2])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[2]), app.ExitUsage)
		}
		pos = p
	case "vm":
		if len(args) < 3 {
			return app.NewExitError(fmt.Errorf("VM target and position are required for VM scope"), app.ExitUsage)
		}
		var err error
		node, vmid, err = parseNodeVMID(args[1])
		if err != nil {
			return app.NewExitError(err, app.ExitUsage)
		}
		p, err := strconv.Atoi(args[2])
		if err != nil || p < 0 {
			return app.NewExitError(fmt.Errorf("invalid position: %s", args[2]), app.ExitUsage)
		}
		pos = p
	default:
		return app.NewExitError(fmt.Errorf("unknown scope: %s (use cluster, node, or vm)", scope), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	var desc, target string
	switch scope {
	case "cluster":
		desc = fmt.Sprintf("firewall rule pos %d (cluster)", pos)
		target = fmt.Sprintf("cluster-rule-%d", pos)
	case "node":
		desc = fmt.Sprintf("firewall rule pos %d (node %s)", pos, node)
		target = fmt.Sprintf("%s-rule-%d", node, pos)
	case "vm":
		desc = fmt.Sprintf("firewall rule pos %d (VM %s/%d)", pos, node, vmid)
		target = fmt.Sprintf("%s-%d-rule-%d", node, vmid, pos)
	}

	if err := checkDestructive(cmdCtx, desc, target); err != nil {
		return err
	}

	switch scope {
	case "cluster":
		err = fm.DeleteFirewallRule(ctx, pos)
	case "node":
		err = fm.DeleteNodeFirewallRule(ctx, node, pos)
	case "vm":
		err = fm.DeleteVMFirewallRule(ctx, node, vmid, pos)
	}
	if err != nil {
		return fmt.Errorf("delete firewall rule: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall rule at position %d deleted\n", pos)
	return nil
}

// --- Firewall alias mutation handlers ---

func runFirewallAliasCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall alias create <name> <cidr> [--comment \"...\"]"), app.ExitUsage)
	}

	name := args[0]
	cidr := args[1]
	var comment string
	for i := 2; i < len(args); i++ {
		if args[i] == "--comment" && i+1 < len(args) {
			comment = args[i+1]
			i++
		}
	}

	if name == "" || cidr == "" {
		return app.NewExitError(fmt.Errorf("name and CIDR are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall alias %s (%s)", name, cidr)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := fm.CreateFirewallAlias(ctx, name, cidr, comment); err != nil {
		return fmt.Errorf("create firewall alias: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall alias %s created\n", name)
	return nil
}

func runFirewallAliasDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall alias delete <name>"), app.ExitUsage)
	}

	name := args[0]
	if name == "" {
		return app.NewExitError(fmt.Errorf("alias name is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall alias %s", name)
	if err := checkDestructive(cmdCtx, desc, name); err != nil {
		return err
	}

	if err := fm.DeleteFirewallAlias(ctx, name); err != nil {
		return fmt.Errorf("delete firewall alias: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall alias %s deleted\n", name)
	return nil
}

// --- Firewall IP set mutation handlers ---

func runFirewallIPSetCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset create <name> [--comment \"...\"]"), app.ExitUsage)
	}

	name := args[0]
	var comment string
	for i := 1; i < len(args); i++ {
		if args[i] == "--comment" && i+1 < len(args) {
			comment = args[i+1]
			i++
		}
	}

	if name == "" {
		return app.NewExitError(fmt.Errorf("IP set name is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall IP set %s", name)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := fm.CreateFirewallIPSet(ctx, name, comment); err != nil {
		return fmt.Errorf("create firewall IP set: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall IP set %s created\n", name)
	return nil
}

func runFirewallIPSetEntryAdd(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset entry add <name> <cidr> [--comment \"...\"]"), app.ExitUsage)
	}

	name := args[0]
	cidr := args[1]
	var comment string
	for i := 2; i < len(args); i++ {
		if args[i] == "--comment" && i+1 < len(args) {
			comment = args[i+1]
			i++
		}
	}

	if name == "" || cidr == "" {
		return app.NewExitError(fmt.Errorf("IP set name and CIDR are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("IP set %s entry %s", name, cidr)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := fm.AddFirewallIPSetEntry(ctx, name, cidr, comment); err != nil {
		return fmt.Errorf("add IP set entry: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Entry %s added to IP set %s\n", cidr, name)
	return nil
}

func runFirewallIPSetEntryRemove(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 2 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset entry remove <name> <cidr>"), app.ExitUsage)
	}

	name := args[0]
	cidr := args[1]
	if name == "" || cidr == "" {
		return app.NewExitError(fmt.Errorf("IP set name and CIDR are required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("IP set %s entry %s", name, cidr)
	target := fmt.Sprintf("%s-%s", name, cidr)
	if err := checkDestructive(cmdCtx, desc, target); err != nil {
		return err
	}

	if err := fm.RemoveFirewallIPSetEntry(ctx, name, cidr); err != nil {
		return fmt.Errorf("remove IP set entry: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Entry %s removed from IP set %s\n", cidr, name)
	return nil
}

func runFirewallIPSetDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset delete <name>"), app.ExitUsage)
	}

	name := args[0]
	if name == "" {
		return app.NewExitError(fmt.Errorf("IP set name is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall IP set %s", name)
	if err := checkDestructive(cmdCtx, desc, name); err != nil {
		return err
	}

	if err := fm.DeleteFirewallIPSet(ctx, name); err != nil {
		return fmt.Errorf("delete firewall IP set: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall IP set %s deleted\n", name)
	return nil
}

// --- Firewall security group mutation handlers ---

func runFirewallGroupCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall group create <name> [--comment \"...\"]"), app.ExitUsage)
	}

	name := args[0]
	var comment string
	for i := 1; i < len(args); i++ {
		if args[i] == "--comment" && i+1 < len(args) {
			comment = args[i+1]
			i++
		}
	}

	if name == "" {
		return app.NewExitError(fmt.Errorf("group name is required"), app.ExitUsage)
	}
	if len(name) > 18 {
		return app.NewExitError(fmt.Errorf("group name %q is too long (maximum 18 characters)", name), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall security group %s", name)
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := fm.CreateFirewallGroup(ctx, name, comment); err != nil {
		return fmt.Errorf("create firewall group: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall group %s created\n", name)
	return nil
}

func runFirewallGroupDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall group delete <name>"), app.ExitUsage)
	}

	name := args[0]
	if name == "" {
		return app.NewExitError(fmt.Errorf("group name is required"), app.ExitUsage)
	}
	if len(name) > 18 {
		return app.NewExitError(fmt.Errorf("group name %q is too long (maximum 18 characters)", name), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("firewall security group %s", name)
	if err := checkDestructive(cmdCtx, desc, name); err != nil {
		return err
	}

	if err := fm.DeleteFirewallGroup(ctx, name); err != nil {
		return fmt.Errorf("delete firewall group: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall group %s deleted\n", name)
	return nil
}

// --- Firewall options update handler ---

func runFirewallOptionsUpdate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex firewall options update enable=<0|1> [policy_in=<a>] [policy_out=<a>] [log_in_drop=<0|1>] [log_ratelimit=<s>] [nf_conntrack_max=<n>] [digest=<s>]"),
			app.ExitUsage)
	}

	var opts domain.FirewallOptionsUpdateInput
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid parameter: %s (expected key=value)", arg), app.ExitUsage)
		}
		key := parts[0]
		val := parts[1]
		switch key {
		case "enable":
			v, err := strconv.Atoi(val)
			if err != nil {
				return app.NewExitError(fmt.Errorf("invalid enable value: %s", val), app.ExitUsage)
			}
			opts.Enable = v
		case "policy_in":
			opts.PolicyIn = val
		case "policy_out":
			opts.PolicyOut = val
		case "log_in_drop":
			v, err := strconv.Atoi(val)
			if err != nil {
				return app.NewExitError(fmt.Errorf("invalid log_in_drop value: %s", val), app.ExitUsage)
			}
			opts.LogInDrop = v
		case "log_ratelimit":
			opts.LogRateLimit = val
		case "nf_conntrack_max":
			v, err := strconv.Atoi(val)
			if err != nil {
				return app.NewExitError(fmt.Errorf("invalid nf_conntrack_max value: %s", val), app.ExitUsage)
			}
			opts.NFConntrack = v
		case "digest":
			opts.Digest = val
		default:
			return app.NewExitError(fmt.Errorf("unknown option: %s", key), app.ExitUsage)
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	fm, err := requireFirewallMutation(prov)
	if err != nil {
		return err
	}

	desc := "firewall options"
	if err := checkDisruptive(cmdCtx, desc); err != nil {
		return err
	}

	if err := fm.UpdateFirewallOptions(ctx, opts); err != nil {
		return fmt.Errorf("update firewall options: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "Firewall options updated\n")
	return nil
}

// --- Access / Identity read-only handlers ---

func runAccessUsersList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access users list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	users, err := ap.Users(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	return writeAccessUsers(cmdCtx, applyLimit(users, cmdCtx.Opts.Limit))
}

func runAccessGroupsList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access groups list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	groups, err := ap.Groups(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}

	return writeAccessGroups(cmdCtx, applyLimit(groups, cmdCtx.Opts.Limit))
}

func runAccessRolesList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access roles list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	roles, err := ap.Roles(ctx)
	if err != nil {
		return fmt.Errorf("list roles: %w", err)
	}

	return writeAccessRoles(cmdCtx, applyLimit(roles, cmdCtx.Opts.Limit))
}

func runAccessACLList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access acl list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	acl, err := ap.ACL(ctx)
	if err != nil {
		return fmt.Errorf("list ACL: %w", err)
	}

	return writeAccessACL(cmdCtx, applyLimit(acl, cmdCtx.Opts.Limit))
}

func runAccessDomainsList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access domains list"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	domains, err := ap.Domains(ctx)
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}

	return writeAccessDomains(cmdCtx, applyLimit(domains, cmdCtx.Opts.Limit))
}

func runAccessTokensList(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex access tokens list <user>"), app.ExitUsage)
	}

	user := args[0]
	if user == "" {
		return app.NewExitError(fmt.Errorf("user ID is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	tokens, err := ap.Tokens(ctx, user)
	if err != nil {
		return fmt.Errorf("list tokens for %s: %w", user, err)
	}

	return writeAccessTokens(cmdCtx, applyLimit(tokens, cmdCtx.Opts.Limit))
}

// --- Access / Identity mutation handlers (Tier 4, expert mode) ---

// checkSecurityAdmin verifies Tier 4 authorization. Returns nil if authorized.
// Requires --expert flag. Prints prompts to stderr when interactive confirmation
// is required but not provided. Returns error if not authorized.
func checkSecurityAdmin(cmdCtx *Context, desc string) error {
	if !cmdCtx.Opts.Expert {
		return app.NewExitError(
			fmt.Errorf("%w: identity operations require --expert flag (Tier 4: Security Administration)", safety.ErrExpertRequired),
			app.ExitUsage,
		)
	}
	policy := safety.ConfirmationPolicy{
		Tier:                safety.TierSecurityAdmin,
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

func runAccessUserCreate(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex access user create <userid> [--password-stdin] [email=<e>] [firstname=<f>] [lastname=<l>] [comment=<c>]"),
			app.ExitUsage)
	}

	userid := args[0]
	var email, firstname, lastname, comment string
	for _, arg := range args[1:] {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return app.NewExitError(fmt.Errorf("invalid parameter: %s (expected key=value)", arg), app.ExitUsage)
		}
		switch parts[0] {
		case "password":
			return app.NewExitError(fmt.Errorf(
				"passwords must not be passed as command arguments; use --password-stdin or interactive prompt"),
				app.ExitUsage)
		case "email":
			email = parts[1]
		case "firstname":
			firstname = parts[1]
		case "lastname":
			lastname = parts[1]
		case "comment":
			comment = parts[1]
		default:
			return app.NewExitError(fmt.Errorf("unknown parameter: %s", parts[0]), app.ExitUsage)
		}
	}

	if userid == "" {
		return app.NewExitError(fmt.Errorf("userid is required"), app.ExitUsage)
	}

	// Secure password collection.
	var password string
	if cmdCtx.Opts.PasswordStdin {
		// Bound stdin reads to prevent memory exhaustion.
		limited := io.LimitReader(cmdCtx.Stdin, 4096)
		data, err := io.ReadAll(limited)
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		password = strings.TrimSpace(string(data))
	} else if !cmdCtx.Opts.NonInteractive {
		reader := bufio.NewReader(os.Stdin)
		pw, err := promptSecret(cmdCtx, reader, "Password: ")
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		password = pw
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("create user %s", userid)
	if err := checkSecurityAdmin(cmdCtx, desc); err != nil {
		return err
	}

	if err := ap.CreateUser(ctx, userid, password, email, firstname, lastname, comment); err != nil {
		return fmt.Errorf("create user %s: %w", userid, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "User %s created\n", userid)
	return nil
}

func runAccessUserDelete(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex access user delete <userid>"), app.ExitUsage)
	}

	userid := args[0]
	if userid == "" {
		return app.NewExitError(fmt.Errorf("userid is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("delete user %s", userid)
	if err := checkSecurityAdmin(cmdCtx, desc); err != nil {
		return err
	}

	if err := ap.DeleteUser(ctx, userid); err != nil {
		return fmt.Errorf("delete user %s: %w", userid, err)
	}

	fmt.Fprintf(cmdCtx.Writer, "User %s deleted\n", userid)
	return nil
}

func runAccessACLAdd(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf(
			"usage: nodex access acl add <path> --role <role> [--user <id>] [--group <id>] [--propagate]"),
			app.ExitUsage)
	}

	path := args[0]
	var role, user, group string
	propagate := 0
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--role":
			if i+1 < len(args) {
				role = args[i+1]
				i++
			}
		case "--user":
			if i+1 < len(args) {
				user = args[i+1]
				i++
			}
		case "--group":
			if i+1 < len(args) {
				group = args[i+1]
				i++
			}
		case "--propagate":
			propagate = 1
		}
	}

	if path == "" {
		return app.NewExitError(fmt.Errorf("ACL path is required"), app.ExitUsage)
	}
	if role == "" {
		return app.NewExitError(fmt.Errorf("role (--role) is required"), app.ExitUsage)
	}
	if user == "" && group == "" {
		return app.NewExitError(fmt.Errorf("either --user or --group is required"), app.ExitUsage)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, cmdCtx.Opts.Profile)
	if err != nil {
		return err
	}
	defer cleanup()

	ap, err := requireAccess(prov)
	if err != nil {
		return err
	}

	desc := fmt.Sprintf("ACL add path=%s role=%s", path, role)
	if err := checkSecurityAdmin(cmdCtx, desc); err != nil {
		return err
	}

	if err := ap.AddACL(ctx, path, role, user, group, propagate); err != nil {
		return fmt.Errorf("add ACL: %w", err)
	}

	fmt.Fprintf(cmdCtx.Writer, "ACL added: path=%s role=%s\n", path, role)
	return nil
}

// --- Access output formatters ---

func writeAccessUsers(cmdCtx *Context, users []domain.AccessUser) error {
	if users == nil {
		users = []domain.AccessUser{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, users)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, users)
	default:
		headers := []string{"USERID", "ENABLED", "EMAIL", "FIRSTNAME", "LASTNAME", "COMMENT"}
		rows := make([][]string, 0, len(users))
		for _, u := range users {
			enabled := "no"
			if u.Enable != 0 {
				enabled = "yes"
			}
			rows = append(rows, []string{u.UserID, enabled, u.Email, u.FirstName, u.LastName, u.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func writeAccessGroups(cmdCtx *Context, groups []domain.AccessGroup) error {
	if groups == nil {
		groups = []domain.AccessGroup{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, groups)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, groups)
	default:
		headers := []string{"GROUPID", "MEMBERS", "COMMENT"}
		rows := make([][]string, 0, len(groups))
		for _, g := range groups {
			rows = append(rows, []string{g.GroupID, fmt.Sprintf("%d", len(g.Members)), g.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func writeAccessRoles(cmdCtx *Context, roles []domain.AccessRole) error {
	if roles == nil {
		roles = []domain.AccessRole{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, roles)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, roles)
	default:
		headers := []string{"ROLEID", "SPECIAL", "PRIVS"}
		rows := make([][]string, 0, len(roles))
		for _, r := range roles {
			special := "no"
			if r.Special != 0 {
				special = "yes"
			}
			rows = append(rows, []string{r.RoleID, special, r.Privs})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func writeAccessACL(cmdCtx *Context, acl []domain.AccessACLEntry) error {
	if acl == nil {
		acl = []domain.AccessACLEntry{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, acl)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, acl)
	default:
		headers := []string{"PATH", "TYPE", "ROLEID", "PROPAGATE", "USERID", "GROUPID"}
		rows := make([][]string, 0, len(acl))
		for _, a := range acl {
			propagate := "no"
			if a.Propagate != 0 {
				propagate = "yes"
			}
			rows = append(rows, []string{a.Path, a.Type, a.RoleID, propagate, a.UserID, a.GroupID})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func writeAccessDomains(cmdCtx *Context, domains []domain.AccessDomain) error {
	if domains == nil {
		domains = []domain.AccessDomain{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, domains)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, domains)
	default:
		headers := []string{"REALM", "TYPE", "DEFAULT", "TFA", "COMMENT"}
		rows := make([][]string, 0, len(domains))
		for _, d := range domains {
			isDefault := "no"
			if d.Default != 0 {
				isDefault = "yes"
			}
			rows = append(rows, []string{d.Realm, d.Type, isDefault, d.TFA, d.Comment})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func writeAccessTokens(cmdCtx *Context, tokens []domain.AccessToken) error {
	if tokens == nil {
		tokens = []domain.AccessToken{}
	}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, tokens)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, tokens)
	default:
		headers := []string{"TOKENID", "DISABLED", "PRIVSEP", "COMMENT", "USERID"}
		rows := make([][]string, 0, len(tokens))
		for _, t := range tokens {
			disabled := "no"
			if t.Disabled != 0 {
				disabled = "yes"
			}
			privsep := "no"
			if t.Privsep != 0 {
				privsep = "yes"
			}
			rows = append(rows, []string{t.TokenID, disabled, privsep, t.Comment, t.UserID})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// --- Network show handler (read-only) ---

// runNetworkShow shows network interfaces for a node (same as node network).
func runNetworkShow(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex network show <node>"), app.ExitUsage)
	}
	return runNodeNetwork(ctx, cmdCtx, args)
}

// --- Firewall dispatch handlers ---

func runFirewallAliasDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex firewall alias <create|delete|list> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  list            List all firewall aliases (default)")
		fmt.Fprintln(cmdCtx.Writer, "  create <name> <cidr> [--comment \"...\"]")
		fmt.Fprintln(cmdCtx.Writer, "  delete <name>")
		return nil
	}
	switch args[0] {
	case "list":
		return runFirewallAliases(ctx, cmdCtx, args[1:])
	case "create":
		return runFirewallAliasCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runFirewallAliasDelete(ctx, cmdCtx, args[1:])
	default:
		// Default: treat first arg as name for backwards compat with aliases show behavior
		// Actually, just default to list behavior
		return app.NewExitError(
			fmt.Errorf("unknown firewall alias subcommand: %s (use list, create, or delete)", args[0]),
			app.ExitUsage,
		)
	}
}

func runFirewallIPSetDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset <name|list|show|create|entry|delete>"), app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runFirewallIPSets(ctx, cmdCtx, args[1:])
	case "show":
		if len(args) < 2 {
			return app.NewExitError(fmt.Errorf("IP set name is required"), app.ExitUsage)
		}
		return runFirewallIPSet(ctx, cmdCtx, args[1:])
	case "create":
		return runFirewallIPSetCreate(ctx, cmdCtx, args[1:])
	case "entry":
		if len(args) < 2 {
			return app.NewExitError(fmt.Errorf("usage: nodex firewall ipset entry <add|remove> [args]"), app.ExitUsage)
		}
		switch args[1] {
		case "add":
			return runFirewallIPSetEntryAdd(ctx, cmdCtx, args[2:])
		case "remove":
			return runFirewallIPSetEntryRemove(ctx, cmdCtx, args[2:])
		default:
			return app.NewExitError(fmt.Errorf("unknown ipset entry subcommand: %s (use add or remove)", args[1]), app.ExitUsage)
		}
	case "delete":
		return runFirewallIPSetDelete(ctx, cmdCtx, args[1:])
	default:
		// Backward compatibility: first arg is the IP set name -> show entries
		return runFirewallIPSet(ctx, cmdCtx, args)
	}
}

func runFirewallGroupDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex firewall group <create|delete|list> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  list            List all security groups (default)")
		fmt.Fprintln(cmdCtx.Writer, "  create <name> [--comment \"...\"]")
		fmt.Fprintln(cmdCtx.Writer, "  delete <name>")
		return nil
	}
	switch args[0] {
	case "list":
		return runFirewallSecurityGroups(ctx, cmdCtx, args[1:])
	case "create":
		return runFirewallGroupCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runFirewallGroupDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown firewall group subcommand: %s (use list, create, or delete)", args[0]),
			app.ExitUsage,
		)
	}
}

func runFirewallOptionsDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		// Backward compatibility: no args shows firewall options
		return runFirewallOptions(ctx, cmdCtx, args)
	}
	switch args[0] {
	case "show":
		return runFirewallOptions(ctx, cmdCtx, args[1:])
	case "update":
		return runFirewallOptionsUpdate(ctx, cmdCtx, args[1:])
	default:
		// Backward compatibility: treat as show
		return runFirewallOptions(ctx, cmdCtx, args)
	}
}

// --- Access dispatch handlers ---

func runAccessUsersDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return runAccessUsersList(ctx, cmdCtx, args)
	}
	switch args[0] {
	case "list":
		return runAccessUsersList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access users subcommand: %s (use list)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessGroupsDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return runAccessGroupsList(ctx, cmdCtx, args)
	}
	switch args[0] {
	case "list":
		return runAccessGroupsList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access groups subcommand: %s (use list)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessRolesDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return runAccessRolesList(ctx, cmdCtx, args)
	}
	switch args[0] {
	case "list":
		return runAccessRolesList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access roles subcommand: %s (use list)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessACLDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex access acl <list|add> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  list            List all ACL entries")
		fmt.Fprintln(cmdCtx.Writer, "  add <path> --role <role> [--user <id>] [--group <id>] [--propagate]")
		return nil
	}
	switch args[0] {
	case "list":
		return runAccessACLList(ctx, cmdCtx, args[1:])
	case "add":
		return runAccessACLAdd(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access acl subcommand: %s (use list or add)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessDomainsDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return runAccessDomainsList(ctx, cmdCtx, args)
	}
	switch args[0] {
	case "list":
		return runAccessDomainsList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access domains subcommand: %s (use list)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessTokensDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex access tokens list <user>"), app.ExitUsage)
	}
	switch args[0] {
	case "list":
		return runAccessTokensList(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access tokens subcommand: %s (use list <user>)", args[0]),
			app.ExitUsage,
		)
	}
}

func runAccessUserDispatch(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "Usage: nodex access user <create|delete> [args]")
		fmt.Fprintln(cmdCtx.Writer, "  create <userid> [--password-stdin] [email=<e>] [firstname=<f>] [lastname=<l>] [comment=<c>]")
		fmt.Fprintln(cmdCtx.Writer, "  delete <userid>")
		fmt.Fprintln(cmdCtx.Writer)
		fmt.Fprintln(cmdCtx.Writer, "Passwords are never accepted as command arguments.")
		fmt.Fprintln(cmdCtx.Writer, "Use --password-stdin for scripting or interactive prompt by default.")
		fmt.Fprintln(cmdCtx.Writer, "Identity operations require --expert flag (Tier 4)")
		return nil
	}
	switch args[0] {
	case "create":
		return runAccessUserCreate(ctx, cmdCtx, args[1:])
	case "delete":
		return runAccessUserDelete(ctx, cmdCtx, args[1:])
	default:
		return app.NewExitError(
			fmt.Errorf("unknown access user subcommand: %s (use create or delete)", args[0]),
			app.ExitUsage,
		)
	}
}
