package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/provider"
	_ "github.com/geoffmcc/nodex/internal/provider/proxmox" // register provider
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func runProviderList(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex provider list"), app.ExitUsage)
	}
	names := provider.List()
	sort.Strings(names)

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		type providerEntry struct {
			Name string `json:"name"`
		}
		var entries []providerEntry
		for _, name := range names {
			entries = append(entries, providerEntry{Name: name})
		}
		return output.WriteJSON(cmdCtx.Writer, entries)

	case output.FormatYAML:
		type providerEntry struct {
			Name string `yaml:"name"`
		}
		var entries []providerEntry
		for _, name := range names {
			entries = append(entries, providerEntry{Name: name})
		}
		return output.WriteYAML(cmdCtx.Writer, entries)

	default:
		headers := []string{"NAME"}
		rows := make([][]string, 0, len(names))
		for _, name := range names {
			rows = append(rows, []string{name})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runProviderCapabilities(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex provider capabilities <name>"),
			app.ExitUsage,
		)
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex provider capabilities <name>"), app.ExitUsage)
	}

	name := config.NormalizeProvider(args[0])
	prov, err := provider.Get(name)
	if err != nil {
		return app.NewExitError(err, app.ExitProvider)
	}

	caps := prov.Capabilities()
	sort.Slice(caps, func(i, j int) bool { return caps[i] < caps[j] })

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		type capEntry struct {
			Capability string `json:"capability"`
		}
		var entries []capEntry
		for _, c := range caps {
			entries = append(entries, capEntry{Capability: string(c)})
		}
		return output.WriteJSON(cmdCtx.Writer, entries)

	case output.FormatYAML:
		type capEntry struct {
			Capability string `yaml:"capability"`
		}
		var entries []capEntry
		for _, c := range caps {
			entries = append(entries, capEntry{Capability: string(c)})
		}
		return output.WriteYAML(cmdCtx.Writer, entries)

	default:
		headers := []string{"CAPABILITY"}
		rows := make([][]string, 0, len(caps))
		for _, c := range caps {
			rows = append(rows, []string{string(c)})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

// connectProfile loads a profile and connects a provider.
// Returns the connected provider and a cleanup function.
func connectProfile(ctx context.Context, cmdCtx *Context, profileName string) (domain.Provider, func(), error) {
	cfg, err := config.Read()
	if err != nil {
		return nil, nil, err
	}

	name := profileName
	if name == "" {
		name = cfg.CurrentProfile
	}
	if name == "" {
		return nil, nil, app.NewExitError(
			fmt.Errorf("%w: no profile specified and no current profile", app.ErrNoProfile),
			app.ExitConfig,
		)
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return nil, nil, app.NewExitError(
			fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name),
			app.ExitConfig,
		)
	}

	if p.Endpoint == "" {
		return nil, nil, app.NewExitError(
			fmt.Errorf("profile %q has no endpoint configured", name),
			app.ExitConfig,
		)
	}

	resolver := credentials.NewResolver("")
	creds, err := resolver.Resolve(ctx, name, p.CredentialRef)
	if err != nil {
		return nil, nil, err
	}

	prov, err := provider.Get(p.Provider)
	if err != nil {
		return nil, nil, app.NewExitError(err, app.ExitProvider)
	}

	opts := []httpclient.Option{httpclient.WithTimeout(cmdCtx.Opts.Timeout)}
	if p.CAFile != "" {
		caOpt, err := httpclient.WithCACert(p.CAFile)
		if err != nil {
			return nil, nil, app.NewExitError(fmt.Errorf("profile %q ca_file: %w", name, err), app.ExitTLS)
		}
		opts = append(opts, caOpt)
	}
	if configurable, ok := prov.(interface {
		ConnectWithOptions(string, *domain.Credentials, ...httpclient.Option) error
	}); ok {
		err = configurable.ConnectWithOptions(p.Endpoint, creds, opts...)
	} else {
		err = prov.Connect(ctx, p.Endpoint, creds)
	}
	if err != nil {
		return nil, nil, app.NewExitError(
			fmt.Errorf("connect to %s: %w", p.Endpoint, err),
			app.ExitNetwork,
		)
	}

	cleanup := func() { _ = prov.Close() }
	return prov, cleanup, nil
}

// formatBytes formats bytes as a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
