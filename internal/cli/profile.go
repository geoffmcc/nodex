package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
)

func runProfileAdd(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile add <name>"),
			app.ExitUsage,
		)
	}

	name := args[0]
	if !config.ProfileRegex.MatchString(name) {
		return app.NewExitError(
			fmt.Errorf("invalid profile name %q (must match %s)", name, config.ProfileRegex),
			app.ExitUsage,
		)
	}

	cfg, err := config.Read()
	if err != nil {
		return err
	}

	if _, exists := cfg.Profiles[name]; exists {
		return app.NewExitError(
			fmt.Errorf("%w: profile %q already exists", app.ErrProfileExists, name),
			app.ExitUsage,
		)
	}

	// If this is the first profile, set it as current.
	if len(cfg.Profiles) == 0 {
		cfg.CurrentProfile = name
	}

	cfg.Profiles[name] = config.Profile{
		Provider: "proxmox",
	}

	if err := config.Write(cfg); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Profile %q added.\n", name)
	}
	return nil
}

func runProfileList(_ context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	names := config.ProfileNames(cfg)
	if len(names) == 0 {
		fmt.Fprintln(cmdCtx.Writer, "No profiles configured.")
		return nil
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		type profileEntry struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
			config.Profile
		}
		var entries []profileEntry
		for _, name := range names {
			p := cfg.Profiles[name]
			entries = append(entries, profileEntry{
				Name:    name,
				Current: name == cfg.CurrentProfile,
				Profile: p,
			})
		}
		return output.WriteJSON(cmdCtx.Writer, entries)

	case output.FormatYAML:
		type profileEntry struct {
			Name    string `yaml:"name"`
			Current bool   `yaml:"current"`
			config.Profile
		}
		var entries []profileEntry
		for _, name := range names {
			p := cfg.Profiles[name]
			entries = append(entries, profileEntry{
				Name:    name,
				Current: name == cfg.CurrentProfile,
				Profile: p,
			})
		}
		return output.WriteYAML(cmdCtx.Writer, entries)

	default:
		headers := []string{"NAME", "PROVIDER", "ENDPOINT", "CURRENT"}
		rows := make([][]string, 0, len(names))
		for _, name := range names {
			p := cfg.Profiles[name]
			current := ""
			if name == cfg.CurrentProfile {
				current = "*"
			}
			rows = append(rows, []string{name, p.Provider, p.Endpoint, current})
		}
		return output.WriteTable(cmdCtx.Writer, headers, rows)
	}
}

func runProfileShow(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile show <name>"),
			app.ExitUsage,
		)
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

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		type profileDetail struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
			config.Profile
		}
		return output.WriteJSON(cmdCtx.Writer, profileDetail{
			Name:    name,
			Current: name == cfg.CurrentProfile,
			Profile: p,
		})

	case output.FormatYAML:
		type profileDetail struct {
			Name    string `yaml:"name"`
			Current bool   `yaml:"current"`
			config.Profile
		}
		return output.WriteYAML(cmdCtx.Writer, profileDetail{
			Name:    name,
			Current: name == cfg.CurrentProfile,
			Profile: p,
		})

	default:
		fmt.Fprintf(cmdCtx.Writer, "Name:      %s\n", name)
		fmt.Fprintf(cmdCtx.Writer, "Provider:  %s\n", p.Provider)
		fmt.Fprintf(cmdCtx.Writer, "Endpoint:  %s\n", p.Endpoint)
		fmt.Fprintf(cmdCtx.Writer, "Cred Ref:  %s\n", p.CredentialRef)
		if p.CAFile != "" {
			fmt.Fprintf(cmdCtx.Writer, "CA File:   %s\n", p.CAFile)
		}
		if name == cfg.CurrentProfile {
			fmt.Fprintln(cmdCtx.Writer, "Current:   yes")
		}
		return nil
	}
}

func runProfileUse(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile use <name>"),
			app.ExitUsage,
		)
	}

	name := args[0]
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	if _, ok := cfg.Profiles[name]; !ok {
		return app.NewExitError(
			fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name),
			app.ExitConfig,
		)
	}

	cfg.CurrentProfile = name
	if err := config.Write(cfg); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Current profile set to %q.\n", name)
	}
	return nil
}

func runProfileCurrent(_ context.Context, cmdCtx *Context, _ []string) error {
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	if cfg.CurrentProfile == "" {
		return app.NewExitError(
			fmt.Errorf("%w: no current profile set", app.ErrNoProfile),
			app.ExitConfig,
		)
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, map[string]string{
			"profile": cfg.CurrentProfile,
		})
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, map[string]string{
			"profile": cfg.CurrentProfile,
		})
	default:
		fmt.Fprintln(cmdCtx.Writer, cfg.CurrentProfile)
		return nil
	}
}

func runProfileTest(_ context.Context, cmdCtx *Context, args []string) error {
	// Phase 3 will implement actual connectivity testing.
	// For now, validate the profile exists and print status.
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	cfg, err := config.Read()
	if err != nil {
		return err
	}

	if name == "" {
		name = cfg.CurrentProfile
	}

	if name == "" {
		return app.NewExitError(
			fmt.Errorf("%w: no profile specified and no current profile", app.ErrNoProfile),
			app.ExitConfig,
		)
	}

	p, ok := cfg.Profiles[name]
	if !ok {
		return app.NewExitError(
			fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name),
			app.ExitConfig,
		)
	}

	fmt.Fprintf(cmdCtx.Writer, "Testing profile %q (%s)...\n", name, p.Endpoint)
	fmt.Fprintln(cmdCtx.Writer, "Connectivity testing not yet implemented (Phase 3).")
	return nil
}

func runProfileRemove(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile remove <name>"),
			app.ExitUsage,
		)
	}

	name := args[0]
	cfg, err := config.Read()
	if err != nil {
		return err
	}

	if _, ok := cfg.Profiles[name]; !ok {
		return app.NewExitError(
			fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name),
			app.ExitConfig,
		)
	}

	// Check for --remove-credential flag.
	removeCred := false
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") && strings.Contains(a, "remove-credential") {
			removeCred = true
			break
		}
	}

	delete(cfg.Profiles, name)

	// If removing the current profile, clear it.
	if cfg.CurrentProfile == name {
		cfg.CurrentProfile = ""
		// Auto-select another profile if available.
		for otherName := range cfg.Profiles {
			cfg.CurrentProfile = otherName
			break
		}
	}

	if err := config.Write(cfg); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Profile %q removed.\n", name)
		if removeCred {
			fmt.Fprintln(cmdCtx.Writer, "Credential removal not yet implemented (Phase 2).")
		}
	}
	return nil
}
