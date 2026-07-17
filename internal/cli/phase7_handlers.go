package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

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

	return output.WriteJSON(cmdCtx.Writer, exp)
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

	// Read JSON from stdin with a reasonable size bound.
	limited := io.LimitReader(stdinReader, 1*1024*1024) // 1 MiB
	data, err := io.ReadAll(limited)
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
		imp.Provider = config.ProviderProxmox
	}
	provider := config.NormalizeProvider(imp.Provider)
	if !config.IsKnownProvider(provider) {
		return app.NewExitError(
			fmt.Errorf("unknown provider %q (known providers: %s)",
				provider, strings.Join(config.KnownProviders(), ", ")),
			app.ExitUsage,
		)
	}

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
// The Error field has been removed; per-profile errors are carried in the
// MultiProfileOutput envelope via ProfileResult.Error.
type aggregatedStatus struct {
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	Version  string `json:"version" yaml:"version"`
	Nodes    int    `json:"nodes" yaml:"nodes"`
	VMs      int    `json:"vms" yaml:"vms"`
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

	out := output.NewMultiProfileOutput[aggregatedStatus]()
	for _, profileName := range names {
		p := cfg.Profiles[profileName]
		start := time.Now()

		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			out.AddFailure(profileName, connErr, time.Since(start))
			continue
		}

		as := aggregatedStatus{
			Endpoint: p.Endpoint,
		}

		// Get cluster info.
		if cl, ok := prov.(domain.ClusterInspector); ok {
			if cluster, err := cl.Cluster(ctx); err == nil && cluster != nil {
				as.Version = cluster.Version
				as.Nodes = cluster.Nodes
			}
		}

		// Count VMs.
		if vi, ok := prov.(domain.VMInspector); ok {
			if vms, err := vi.VMs(ctx); err == nil {
				as.VMs = len(vms)
			}
		}

		cleanup()
		out.AddSuccess(profileName, as, time.Since(start))
	}

	out.SortResults()
	_ = writeAggregatedStatus(cmdCtx, out)
	return exitFromMulti(out)
}

func writeAggregatedStatus(cmdCtx *Context, out output.MultiProfileOutput[aggregatedStatus]) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, out)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, out)
	default:
		headers := []string{"PROFILE", "ENDPOINT", "VERSION", "NODES", "VMS"}
		rows := make([][]string, 0, len(out.Results))
		for _, r := range out.Results {
			if !r.Success && r.Error != nil {
				rows = append(rows, []string{r.Profile, r.Data.Endpoint, "", "", fmt.Sprintf("ERROR: %s", r.Error.Detail)})
				continue
			}
			rows = append(rows, []string{r.Profile, r.Data.Endpoint, r.Data.Version,
				fmt.Sprintf("%d", r.Data.Nodes), fmt.Sprintf("%d", r.Data.VMs)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === Nodes --all ===

func runNodesAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	out := output.NewMultiProfileOutput[[]domain.Node]()
	for _, profileName := range names {
		start := time.Now()

		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			out.AddFailure(profileName, connErr, time.Since(start))
			continue
		}

		if ni, ok := prov.(domain.NodeInspector); ok {
			nodes, err := ni.Nodes(ctx)
			if err != nil {
				fmt.Fprintf(cmdCtx.ErrW, "profile %q nodes: %v\n", profileName, err)
				cleanup()
				out.AddFailure(profileName, err, time.Since(start))
				continue
			}
			out.AddSuccess(profileName, applyLimitN(nodes, cmdCtx.Opts.Limit), time.Since(start))
		}
		cleanup()
	}

	out.SortResults()
	_ = writeNodesAll(cmdCtx, out)
	return exitFromMulti(out)
}

func writeNodesAll(cmdCtx *Context, out output.MultiProfileOutput[[]domain.Node]) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, out)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, out)
	default:
		headers := []string{"PROFILE", "NAME", "STATUS", "IP", "ROLE", "UPTIME"}
		var rows [][]string
		for _, r := range out.Results {
			if !r.Success {
				errDetail := ""
				if r.Error != nil {
					errDetail = r.Error.Detail
				}
				rows = append(rows, []string{r.Profile, "", "", "", "", fmt.Sprintf("ERROR: %s", errDetail)})
				continue
			}
			sort.Slice(r.Data, func(i, j int) bool { return r.Data[i].Name < r.Data[j].Name })
			for _, n := range r.Data {
				uptime := ""
				if n.Uptime != nil {
					uptime = n.Uptime.String()
				}
				rows = append(rows, []string{r.Profile, n.Name, n.Status, n.IP, n.Role, uptime})
			}
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === VMs --all ===

func runVMsAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	out := output.NewMultiProfileOutput[[]domain.VM]()
	for _, profileName := range names {
		start := time.Now()

		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			out.AddFailure(profileName, connErr, time.Since(start))
			continue
		}

		if vi, ok := prov.(domain.VMInspector); ok {
			vms, err := vi.VMs(ctx)
			if err != nil {
				fmt.Fprintf(cmdCtx.ErrW, "profile %q vms: %v\n", profileName, err)
				cleanup()
				out.AddFailure(profileName, err, time.Since(start))
				continue
			}
			out.AddSuccess(profileName, applyLimitN(vms, cmdCtx.Opts.Limit), time.Since(start))
		}
		cleanup()
	}

	out.SortResults()
	_ = writeVMsAll(cmdCtx, out)
	return exitFromMulti(out)
}

func writeVMsAll(cmdCtx *Context, out output.MultiProfileOutput[[]domain.VM]) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, out)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, out)
	default:
		headers := []string{"PROFILE", "ID", "NAME", "STATUS", "NODE", "CPU", "MEMORY", "DISK"}
		var rows [][]string
		for _, r := range out.Results {
			if !r.Success {
				errDetail := ""
				if r.Error != nil {
					errDetail = r.Error.Detail
				}
				rows = append(rows, []string{r.Profile, "", "", "", "", "", "", fmt.Sprintf("ERROR: %s", errDetail)})
				continue
			}
			sort.Slice(r.Data, func(i, j int) bool { return r.Data[i].Name < r.Data[j].Name })
			for _, v := range r.Data {
				rows = append(rows, []string{r.Profile, v.ID, v.Name, v.Status, v.Node,
					fmt.Sprintf("%d", v.CPU), formatBytes(v.Memory), formatBytes(v.Disk)})
			}
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// === Containers --all ===

func runContainersAll(ctx context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		return app.NewExitError(fmt.Errorf("no profiles configured"), app.ExitConfig)
	}

	out := output.NewMultiProfileOutput[[]domain.Container]()
	for _, profileName := range names {
		start := time.Now()

		prov, cleanup, connErr := connectProfile(ctx, cmdCtx, profileName)
		if connErr != nil {
			fmt.Fprintf(cmdCtx.ErrW, "profile %q: %v\n", profileName, connErr)
			out.AddFailure(profileName, connErr, time.Since(start))
			continue
		}

		if ci, ok := prov.(domain.ContainerInspector); ok {
			cts, err := ci.Containers(ctx)
			if err != nil {
				fmt.Fprintf(cmdCtx.ErrW, "profile %q containers: %v\n", profileName, err)
				cleanup()
				out.AddFailure(profileName, err, time.Since(start))
				continue
			}
			out.AddSuccess(profileName, applyLimitN(cts, cmdCtx.Opts.Limit), time.Since(start))
		}
		cleanup()
	}

	out.SortResults()
	_ = writeContainersAll(cmdCtx, out)
	return exitFromMulti(out)
}

func writeContainersAll(cmdCtx *Context, out output.MultiProfileOutput[[]domain.Container]) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, out)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, out)
	default:
		headers := []string{"PROFILE", "ID", "NAME", "STATUS", "NODE", "OS", "MEMORY", "DISK"}
		var rows [][]string
		for _, r := range out.Results {
			if !r.Success {
				errDetail := ""
				if r.Error != nil {
					errDetail = r.Error.Detail
				}
				rows = append(rows, []string{r.Profile, "", "", "", "", "", "", fmt.Sprintf("ERROR: %s", errDetail)})
				continue
			}
			sort.Slice(r.Data, func(i, j int) bool { return r.Data[i].Name < r.Data[j].Name })
			for _, c := range r.Data {
				rows = append(rows, []string{r.Profile, c.ID, c.Name, c.Status, c.Node,
					c.OS, formatBytes(c.Memory), formatBytes(c.Disk)})
			}
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// exitFromMulti returns an appropriate error for the process exit code.
// All success → nil (exit 0). Any failure → ExitPartialFailure (exit 11).
func exitFromMulti[T any](out output.MultiProfileOutput[T]) error {
	if out.Failed() == 0 {
		return nil
	}
	if out.AllFailed() {
		return app.NewExitError(
			fmt.Errorf("all %d profile(s) failed", out.Summary.Total),
			app.ExitPartialFailure,
		)
	}
	return app.NewExitError(
		fmt.Errorf("%d of %d profile(s) failed", out.Summary.Failed, out.Summary.Total),
		app.ExitPartialFailure,
	)
}

// applyLimitN applies the limit to any slice type.
func applyLimitN[S any](items []S, limit int) []S {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
