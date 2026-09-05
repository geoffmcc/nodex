package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

type setupInput struct {
	provider, profile, endpoint, credentialRef, caFile string
	check                                              bool
}

func runSetup(ctx context.Context, cmdCtx *Context, args []string) error {
	in, err := parseSetupArgs(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if !cmdCtx.Opts.NonInteractive {
		if err := promptSetup(cmdCtx, &in); err != nil {
			return app.NewExitError(err, app.ExitUsage)
		}
	} else if in.provider == "" || in.profile == "" || in.endpoint == "" {
		return app.NewExitError(fmt.Errorf("non-interactive setup requires --provider, --profile, and --endpoint"), app.ExitUsage)
	}

	in.provider = config.NormalizeProvider(in.provider)
	if !config.IsKnownProvider(in.provider) {
		return app.NewExitError(fmt.Errorf("unknown provider %q (known providers: %s)", in.provider, strings.Join(config.KnownProviders(), ", ")), app.ExitUsage)
	}
	if !config.ProfileRegex.MatchString(in.profile) {
		return app.NewExitError(fmt.Errorf("invalid profile name (must match %s)", config.ProfileRegex), app.ExitUsage)
	}
	if err := config.ValidateEndpoint(in.endpoint); err != nil {
		return app.NewExitError(fmt.Errorf("invalid endpoint: %w", err), app.ExitUsage)
	}
	if in.credentialRef != "" {
		if _, _, err := credentials.ParseCredentialRefStrict(in.credentialRef); err != nil {
			return app.NewExitError(fmt.Errorf("invalid credential reference: %w", err), app.ExitUsage)
		}
	}
	if in.caFile != "" {
		if _, err := httpclient.WithCACert(in.caFile); err != nil {
			return app.NewExitError(fmt.Errorf("invalid CA file: %w", err), app.ExitTLS)
		}
	}

	profile := config.Profile{Provider: in.provider, Endpoint: in.endpoint, CredentialRef: in.credentialRef, CAFile: in.caFile}
	if in.check {
		checks := diagnoseProfile(ctx, cmdCtx, in.profile, profile)
		if err := writePermissionChecks(cmdCtx, in.profile, checks); err != nil {
			return err
		}
		if !setupChecksConfirmed(checks) {
			return app.NewExitError(fmt.Errorf("setup preflight failed; profile was not written"), app.ExitValidationError)
		}
	}
	if _, statErr := os.Stat(configPathForSetup()); statErr == nil {
		existing, err := config.Read()
		if err != nil {
			return err
		}
		if _, exists := existing.Profiles[in.profile]; exists && !cmdCtx.Opts.Force {
			return app.NewExitError(fmt.Errorf("profile %q already exists; use --force to replace it", in.profile), app.ExitConflict)
		}
	} else if !os.IsNotExist(statErr) {
		return app.NewExitError(fmt.Errorf("inspect config: %w", statErr), app.ExitConfig)
	}
	if err := writeSetupProfile(profile, in.profile); err != nil {
		return err
	}

	if in.check {
		return nil
	}
	if !cmdCtx.Opts.Quiet {
		path, _ := config.ConfigPath()
		switch cmdCtx.Opts.Output {
		case output.FormatJSON:
			return output.WriteJSON(cmdCtx.Writer, map[string]string{"profile": in.profile, "config": path})
		case output.FormatYAML:
			return output.WriteYAML(cmdCtx.Writer, map[string]string{"profile": in.profile, "config": path})
		default:
			fmt.Fprintf(cmdCtx.Writer, "Profile %q configured in %s.\n", in.profile, path)
		}
	}
	return nil
}

func setupChecksConfirmed(checks []permissionCheck) bool {
	for _, check := range checks {
		if check.Status != permissionConfirmed {
			return false
		}
	}
	return len(checks) > 0
}

func configPathForSetup() string {
	path, _ := config.ConfigPath()
	return path
}

func writeSetupProfile(profile config.Profile, name string) error {
	path := configPathForSetup()
	var cfg *config.Config
	if _, err := os.Stat(path); os.IsNotExist(err) {
		cfg = config.DefaultConfig()
	} else if err != nil {
		return app.NewExitError(fmt.Errorf("inspect config: %w", err), app.ExitConfig)
	} else {
		var err error
		cfg, err = config.Read()
		if err != nil {
			return err
		}
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]config.Profile)
	}
	cfg.Profiles[name] = profile
	cfg.CurrentProfile = name
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return config.Write(cfg)
	}
	return config.Write(cfg)
}

func parseSetupArgs(args []string) (setupInput, error) {
	var in setupInput
	usage := fmt.Errorf("usage: nodex setup [--provider name] [--profile name] [--endpoint https://host:port] [--credential-ref backend:name] [--ca-file path] [--check]")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--check" {
			in.check = true
			continue
		}
		name, value, ok := strings.Cut(arg, "=")
		if !ok {
			name = arg
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return in, usage
			}
			value = args[i+1]
			i++
		}
		switch name {
		case "--provider":
			in.provider = value
		case "--profile":
			in.profile = value
		case "--endpoint":
			in.endpoint = value
		case "--credential-ref":
			in.credentialRef = value
		case "--ca-file":
			in.caFile = value
		case "--token", "--token-id", "--token-secret", "--password", "--secret":
			return in, fmt.Errorf("setup does not accept secrets as command-line arguments; use a credential reference")
		default:
			return in, usage
		}
	}
	return in, nil
}

func promptSetup(cmdCtx *Context, in *setupInput) error {
	reader := bufio.NewReader(cmdCtx.Stdin)
	var err error
	if in.provider == "" {
		in.provider, err = setupPrompt(reader, cmdCtx.ErrW, "Provider (proxmox|pbs) [proxmox]: ")
		if err != nil {
			return err
		}
		if in.provider == "" {
			in.provider = config.ProviderProxmox
		}
	}
	if in.profile == "" {
		in.profile, err = setupPrompt(reader, cmdCtx.ErrW, "Profile name [default]: ")
		if err != nil {
			return err
		}
		if in.profile == "" {
			in.profile = "default"
		}
	}
	if in.endpoint == "" {
		in.endpoint, err = setupPrompt(reader, cmdCtx.ErrW, "HTTPS endpoint: ")
		if err != nil {
			return err
		}
	}
	if in.credentialRef == "" {
		in.credentialRef, err = setupPrompt(reader, cmdCtx.ErrW, "Credential reference (optional, e.g. keyring:production): ")
		if err != nil {
			return err
		}
	}
	if in.caFile == "" {
		in.caFile, err = setupPrompt(reader, cmdCtx.ErrW, "CA file (optional): ")
		if err != nil {
			return err
		}
	}
	return nil
}

func setupPrompt(reader *bufio.Reader, w io.Writer, text string) (string, error) {
	fmt.Fprint(w, text)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
