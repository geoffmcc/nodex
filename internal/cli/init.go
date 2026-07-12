package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
)

func runInit(_ context.Context, cmdCtx *Context, args []string) error {
	// Check if config already exists.
	path, err := config.ConfigPath()
	if err != nil {
		return app.NewExitError(err, app.ExitConfig)
	}

	if _, statErr := os.Stat(path); statErr == nil {
		if cmdCtx.Opts.NonInteractive {
			return app.NewExitError(
				fmt.Errorf("config already exists at %s", path),
				app.ExitConfig,
			)
		}
		fmt.Fprintf(cmdCtx.ErrW, "Config already exists at %s\n", path)
		fmt.Fprint(cmdCtx.ErrW, "Overwrite? [y/N] ")
		if !confirm(cmdCtx) {
			fmt.Fprintln(cmdCtx.ErrW, "Aborted.")
			return nil
		}
	}

	cfg := config.DefaultConfig()

	if !cmdCtx.Opts.NonInteractive {
		fmt.Fprint(cmdCtx.ErrW, "Provider (e.g. proxmox): ")
		provider := prompt(cmdCtx)
		if provider == "" {
			provider = "proxmox"
		}

		fmt.Fprint(cmdCtx.ErrW, "Endpoint URL: ")
		endpoint := prompt(cmdCtx)
		if endpoint == "" {
			return app.NewExitError(
				fmt.Errorf("endpoint is required"),
				app.ExitUsage,
			)
		}

		fmt.Fprint(cmdCtx.ErrW, "Credential reference (e.g. file:default): ")
		credRef := prompt(cmdCtx)

		fmt.Fprint(cmdCtx.ErrW, "Profile name [default]: ")
		name := prompt(cmdCtx)
		if name == "" {
			name = "default"
		}

		if !config.ProfileRegex.MatchString(name) {
			return app.NewExitError(
				fmt.Errorf("invalid profile name %q (must match %s)", name, config.ProfileRegex),
				app.ExitUsage,
			)
		}

		profile := config.Profile{
			Provider:      config.NormalizeProvider(provider),
			Endpoint:      endpoint,
			CredentialRef: credRef,
		}

		cfg.CurrentProfile = name
		cfg.Profiles[name] = profile
	} else {
		// Non-interactive: create minimal config.
		cfg.CurrentProfile = "default"
		cfg.Profiles["default"] = config.Profile{
			Provider: "proxmox",
		}
	}

	if err := config.Write(cfg); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		configPath, _ := config.ConfigPath()
		fmt.Fprintf(cmdCtx.Writer, "Configuration written to %s\n", configPath)
	}

	return nil
}

func prompt(cmdCtx *Context) string {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func confirm(cmdCtx *Context) bool {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}


