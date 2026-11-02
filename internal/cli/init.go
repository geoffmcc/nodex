package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
)

func runInit(_ context.Context, cmdCtx *Context, args []string) error {
	var keyOpts sshKeyOptions
	for _, arg := range args {
		switch arg {
		case "--generate-ssh-key":
			keyOpts.generate = true
		case "--ssh-key-no-passphrase":
			keyOpts.noPassphrase = true
		default:
			return app.NewExitError(fmt.Errorf("usage: nodex init [--generate-ssh-key] [--ssh-key-no-passphrase]"), app.ExitUsage)
		}
	}
	if err := keyOpts.validate(); err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	reader := bufio.NewReader(cmdCtx.Stdin)
	ask := func(label string) string { value, _ := setupPrompt(reader, cmdCtx.ErrW, label); return value }
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
		answer := ask("")
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(cmdCtx.ErrW, "Aborted.")
			return nil
		}
	}

	cfg := config.DefaultConfig()

	if !cmdCtx.Opts.NonInteractive {
		fmt.Fprintf(cmdCtx.ErrW, "Provider (%s) [proxmox]: ", strings.Join(config.KnownProviders(), "|"))
		provider := ask("")
		if provider == "" {
			provider = config.ProviderProxmox
		}
		provider = config.NormalizeProvider(provider)
		if !config.IsKnownProvider(provider) {
			return app.NewExitError(
				fmt.Errorf("unknown provider %q (known providers: %s)",
					provider, strings.Join(config.KnownProviders(), ", ")),
				app.ExitUsage,
			)
		}

		fmt.Fprint(cmdCtx.ErrW, "Endpoint URL: ")
		endpoint := ask("")
		if endpoint == "" {
			return app.NewExitError(
				fmt.Errorf("endpoint is required"),
				app.ExitUsage,
			)
		}

		fmt.Fprint(cmdCtx.ErrW, "Credential reference (e.g. file:default): ")
		credRef := ask("")
		if credRef != "" {
			if _, _, err := credentials.ParseCredentialRefStrict(credRef); err != nil {
				return app.NewExitError(fmt.Errorf("invalid credential reference: %w", err), app.ExitUsage)
			}
		}

		fmt.Fprint(cmdCtx.ErrW, "Profile name [default]: ")
		name := ask("")
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
			Provider:      provider,
			Endpoint:      endpoint,
			CredentialRef: credRef,
		}
		if err := config.ValidateEndpoint(profile.Endpoint); err != nil {
			return app.NewExitError(fmt.Errorf("invalid endpoint: %w", err), app.ExitUsage)
		}

		cfg.CurrentProfile = name
		cfg.Profiles[name] = profile
	} else {
		// Non-interactive: create minimal config.
		cfg.CurrentProfile = "default"
		cfg.Profiles["default"] = config.Profile{
			Provider: config.ProviderProxmox,
		}
	}

	if err := setupSSHKey(cmdCtx, reader, keyOpts); err != nil {
		return err
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

func prompt() string {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func confirm() bool {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
