package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// === Profile Export ===

// exportedProfile is the sanitized JSON representation for profile export.
type exportedProfile struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	CAFile   string `json:"ca_file,omitempty"`
}

func runProfileExport(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile export <name>"), app.ExitUsage)
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile export <name>"), app.ExitUsage)
	}

	name := args[0]
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return app.NewExitError(
			fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name),
			app.ExitConfig,
		)
	}

	exp := exportedProfile{
		Name:     name,
		Provider: p.Provider,
		Endpoint: p.Endpoint,
		CAFile:   p.CAFile,
	}

	enc := json.NewEncoder(cmdCtx.Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(exp)
}

// === Profile Import ===

// stdinReader allows tests to inject input for commands that read from stdin.
var stdinReader io.Reader = os.Stdin

func runProfileImport(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile import <name>"), app.ExitUsage)
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile import <name>"), app.ExitUsage)
	}

	name := args[0]
	if !config.ProfileRegex.MatchString(name) {
		return app.NewExitError(
			fmt.Errorf("invalid profile name %q (must match %s)", name, config.ProfileRegex),
			app.ExitUsage,
		)
	}

	// Read JSON from stdin.
	data, err := io.ReadAll(stdinReader)
	if err != nil {
		return app.NewExitError(
			fmt.Errorf("read stdin: %w", err),
			app.ExitUsage,
		)
	}

	var imp exportedProfile
	if err := json.Unmarshal(data, &imp); err != nil {
		return app.NewExitError(
			fmt.Errorf("invalid JSON input: %w", err),
			app.ExitUsage,
		)
	}

	if imp.Provider == "" {
		imp.Provider = "proxmox"
	}
	provider := config.NormalizeProvider(imp.Provider)

	if imp.Endpoint != "" {
		if err := config.ValidateEndpoint(imp.Endpoint); err != nil {
			return app.NewExitError(
				fmt.Errorf("imported endpoint: %w", err),
				app.ExitUsage,
			)
		}
	}

	// Create profile in config.
	if err := config.Update(func(cfg *config.Config) error {
		if _, exists := cfg.Profiles[name]; exists {
			return app.NewExitError(
				fmt.Errorf("%w: profile %q already exists", app.ErrProfileExists, name),
				app.ExitConfig,
			)
		}
		if len(cfg.Profiles) == 0 {
			cfg.CurrentProfile = name
		}
		cfg.Profiles[name] = config.Profile{
			Provider: provider,
			Endpoint: imp.Endpoint,
			CAFile:   imp.CAFile,
		}
		return nil
	}); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Profile %q imported.\n", name)
	}
	return nil
}

// === Cross-Profile Aggregation ===

// aggregatedStatus is a per-profile status summary for --all output.
type aggregatedStatus struct {
	Profile  string `json:"profile" yaml:"profile"`
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	Version  string `json:"version" yaml:"version"`
	Nodes    int    `json:"nodes" yaml:"nodes"`
	VMs      int    `json:"vms" yaml:"vms"`
	Error    string `json:"error,omitempty" yaml:"error,omitempty"`
}

func runStatusAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	var results []aggregatedStatus
	for _, profileName := range names {
		p := cfg.Profiles[profileName]
		as := aggregatedStatus{
			Profile:  profileName,
			Endpoint: p.Endpoint,
		}

		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			as.Error = connErr.Error()
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			results = append(results, as)
			continue
		}

		// Get cluster info.
		cluster, err := prov.Cluster(ctx)
		if err == nil && cluster != nil {
			as.Version = cluster.Version
			as.Nodes = cluster.Nodes
		}

		// Count VMs.
		vms, err := prov.VMs(ctx)
		if err == nil {
			as.VMs = len(vms)
		}

		cleanup()
		results = append(results, as)
	}

	return writeAggregatedStatus(cmdCtx, results)
}

func writeAggregatedStatus(cmdCtx *Context, results []aggregatedStatus) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, results)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, results)
	default:
		headers := []string{"PROFILE", "ENDPOINT", "VERSION", "NODES", "VMS"}
		rows := make([][]string, 0, len(results))
		for _, r := range results {
			if r.Error != "" {
				rows = append(rows, []string{r.Profile, r.Endpoint, "", "", fmt.Sprintf("ERROR: %s", r.Error)})
				continue
			}
			rows = append(rows, []string{r.Profile, r.Endpoint, r.Version, fmt.Sprintf("%d", r.Nodes), fmt.Sprintf("%d", r.VMs)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === Nodes --all ===

// nodeWithProfile adds a profile field to domain.Node.
type nodeWithProfile struct {
	Profile string `json:"profile" yaml:"profile"`
	domain.Node
}

func runNodesAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	var allNodes []nodeWithProfile
	for _, profileName := range names {
		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			continue
		}

		nodes, err := prov.Nodes(ctx)
		if err != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q nodes: %v\n", profileName, err)
			cleanup()
			continue
		}

		for _, n := range nodes {
			allNodes = append(allNodes, nodeWithProfile{Profile: profileName, Node: n})
		}
		cleanup()
	}

	return writeNodesAll(cmdCtx, applyLimitN(allNodes, cmdCtx.Opts.Limit))
}

func writeNodesAll(cmdCtx *Context, nodes []nodeWithProfile) error {
	if nodes == nil {
		nodes = []nodeWithProfile{}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Profile != nodes[j].Profile {
			return nodes[i].Profile < nodes[j].Profile
		}
		return nodes[i].Name < nodes[j].Name
	})

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, nodes)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, nodes)
	default:
		headers := []string{"PROFILE", "NAME", "STATUS", "IP", "ROLE", "UPTIME"}
		rows := make([][]string, 0, len(nodes))
		for _, n := range nodes {
			uptime := ""
			if n.Uptime != nil {
				uptime = n.Uptime.String()
			}
			rows = append(rows, []string{n.Profile, n.Name, n.Status, n.IP, n.Role, uptime})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === VMs --all ===

type vmWithProfile struct {
	Profile string `json:"profile" yaml:"profile"`
	domain.VM
}

func runVMsAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	var allVMs []vmWithProfile
	for _, profileName := range names {
		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			continue
		}

		vms, err := prov.VMs(ctx)
		if err != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q vms: %v\n", profileName, err)
			cleanup()
			continue
		}

		for _, v := range vms {
			allVMs = append(allVMs, vmWithProfile{Profile: profileName, VM: v})
		}
		cleanup()
	}

	return writeVMsAll(cmdCtx, applyLimitN(allVMs, cmdCtx.Opts.Limit))
}

func writeVMsAll(cmdCtx *Context, vms []vmWithProfile) error {
	if vms == nil {
		vms = []vmWithProfile{}
	}
	sort.Slice(vms, func(i, j int) bool {
		if vms[i].Profile != vms[j].Profile {
			return vms[i].Profile < vms[j].Profile
		}
		return vms[i].Name < vms[j].Name
	})

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, vms)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, vms)
	default:
		headers := []string{"PROFILE", "ID", "NAME", "STATUS", "NODE", "CPU", "MEMORY", "DISK"}
		rows := make([][]string, 0, len(vms))
		for _, v := range vms {
			rows = append(rows, []string{v.Profile, v.ID, v.Name, v.Status, v.Node,
				fmt.Sprintf("%d", v.CPU), formatBytes(v.Memory), formatBytes(v.Disk)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === Containers --all ===

type containerWithProfile struct {
	Profile string `json:"profile" yaml:"profile"`
	domain.Container
}

func runContainersAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	var allCTs []containerWithProfile
	for _, profileName := range names {
		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			continue
		}

		cts, err := prov.Containers(ctx)
		if err != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q containers: %v\n", profileName, err)
			cleanup()
			continue
		}

		for _, c := range cts {
			allCTs = append(allCTs, containerWithProfile{Profile: profileName, Container: c})
		}
		cleanup()
	}

	return writeContainersAll(cmdCtx, applyLimitN(allCTs, cmdCtx.Opts.Limit))
}

func writeContainersAll(cmdCtx *Context, containers []containerWithProfile) error {
	if containers == nil {
		containers = []containerWithProfile{}
	}
	sort.Slice(containers, func(i, j int) bool {
		if containers[i].Profile != containers[j].Profile {
			return containers[i].Profile < containers[j].Profile
		}
		return containers[i].Name < containers[j].Name
	})

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, containers)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, containers)
	default:
		headers := []string{"PROFILE", "ID", "NAME", "STATUS", "NODE", "OS", "MEMORY", "DISK"}
		rows := make([][]string, 0, len(containers))
		for _, c := range containers {
			rows = append(rows, []string{c.Profile, c.ID, c.Name, c.Status, c.Node,
				c.OS, formatBytes(c.Memory), formatBytes(c.Disk)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// applyLimitN applies the limit to any slice type.
func applyLimitN[S any](items []S, limit int) []S {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
