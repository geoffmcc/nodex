package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"golang.org/x/term"
)

var credentialPrompter = promptCredentials

func runProfileAdd(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile add <name>"),
			app.ExitUsage,
		)
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile add <name>"), app.ExitUsage)
	}

	name := args[0]
	if !config.ProfileRegex.MatchString(name) {
		return app.NewExitError(
			fmt.Errorf("invalid profile name %q (must match %s)", name, config.ProfileRegex),
			app.ExitUsage,
		)
	}

	if err := config.Update(func(cfg *config.Config) error {
		if _, exists := cfg.Profiles[name]; exists {
			return app.NewExitError(fmt.Errorf("%w: profile %q already exists", app.ErrProfileExists, name), app.ExitUsage)
		}
		if len(cfg.Profiles) == 0 {
			cfg.CurrentProfile = name
		}
		cfg.Profiles[name] = config.Profile{Provider: "proxmox"}
		return nil
	}); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Profile %q added.\n", name)
	}
	return nil
}

func runProfileList(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile list"), app.ExitUsage)
	}
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
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile show <name>"), app.ExitUsage)
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

func runProfileSetCredentials(ctx context.Context, cmdCtx *Context, args []string) error {
	name, backendName, storeName, err := parseSetCredentialArgs(args)
	if err != nil {
		return app.NewExitError(err, app.ExitUsage)
	}
	if cmdCtx.Opts.NonInteractive {
		return app.NewExitError(fmt.Errorf("profile set-credentials requires an interactive terminal"), app.ExitUsage)
	}

	if !config.ProfileRegex.MatchString(name) {
		return app.NewExitError(
			fmt.Errorf("invalid profile name %q (must match %s)", name, config.ProfileRegex),
			app.ExitUsage,
		)
	}

	backendNameNormalized := strings.ToLower(strings.TrimSpace(backendName))
	if backendNameNormalized != "file" && backendNameNormalized != "keyring" {
		return app.NewExitError(fmt.Errorf("unsupported credential backend %q (supported: file, keyring)", backendName), app.ExitUsage)
	}

	storeName = strings.TrimSpace(storeName)
	if storeName == "" {
		storeName = name
	}
	if err := credentials.ValidateName(storeName); err != nil {
		return app.NewExitError(fmt.Errorf("credential name: %w", err), app.ExitUsage)
	}

	cfg, err := config.Read()
	if err != nil {
		return err
	}
	if _, ok := cfg.Profiles[name]; !ok {
		return app.NewExitError(fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name), app.ExitConfig)
	}

	creds, err := credentialPrompter(cmdCtx)
	if err != nil {
		return app.NewExitError(fmt.Errorf("read credentials: %w", err), app.ExitCredential)
	}
	if err := credentials.ValidateCredentials(name, creds); err != nil {
		return app.NewExitError(err, app.ExitCredential)
	}

	resolver := credentials.NewResolver("")
	backend, ok := resolver.GetBackend(backendNameNormalized)
	if !ok {
		return app.NewExitError(fmt.Errorf("unknown credential backend %q", backendNameNormalized), app.ExitCredential)
	}
	if err := backend.Store(ctx, storeName, creds); err != nil {
		return app.NewExitError(fmt.Errorf("store credentials in %s backend: %w", backendNameNormalized, err), app.ExitCredential)
	}

	credentialRef := backendNameNormalized + ":" + storeName
	if err := config.Update(func(cfg *config.Config) error {
		p, ok := cfg.Profiles[name]
		if !ok {
			return app.NewExitError(fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name), app.ExitConfig)
		}
		p.CredentialRef = credentialRef
		cfg.Profiles[name] = p
		return nil
	}); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Credentials for profile %q stored in %s:%s.\n", name, backendNameNormalized, storeName)
	}
	return nil
}

func parseSetCredentialArgs(args []string) (name, backendName, credentialName string, err error) {
	backendName = "file"
	usage := fmt.Errorf("usage: nodex profile set-credentials <name> [--backend file|keyring] [--credential-name name]")
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--backend" || arg == "--credential-name":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", "", "", usage
			}
			if arg == "--backend" {
				backendName = args[i+1]
			} else {
				credentialName = args[i+1]
			}
			i++
		case strings.HasPrefix(arg, "--backend="):
			backendName = strings.TrimPrefix(arg, "--backend=")
			if backendName == "" {
				return "", "", "", usage
			}
		case strings.HasPrefix(arg, "--credential-name="):
			credentialName = strings.TrimPrefix(arg, "--credential-name=")
			if credentialName == "" {
				return "", "", "", usage
			}
		case strings.HasPrefix(arg, "--"):
			return "", "", "", usage
		default:
			if name != "" {
				return "", "", "", usage
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", "", usage
	}
	return name, backendName, credentialName, nil
}

func promptCredentials(cmdCtx *Context) (*domain.Credentials, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprint(cmdCtx.ErrW, "Token ID: ")
	tokenID, err := readPromptLine(reader)
	if err != nil {
		return nil, err
	}
	tokenSecret, err := promptSecret(cmdCtx, reader, "Token Secret: ")
	if err != nil {
		return nil, err
	}
	return &domain.Credentials{Type: "token", TokenID: tokenID, TokenSecret: tokenSecret}, nil
}

func promptSecret(cmdCtx *Context, reader *bufio.Reader, promptText string) (string, error) {
	fmt.Fprint(cmdCtx.ErrW, promptText)
	fd := int(os.Stdin.Fd())
	info, statErr := os.Stdin.Stat()
	if statErr == nil && info.Mode()&os.ModeCharDevice != 0 && term.IsTerminal(fd) {
		secret, err := term.ReadPassword(fd)
		fmt.Fprintln(cmdCtx.ErrW)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(secret)), nil
	}
	secret, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", errors.New("token secret is empty")
	}
	return secret, nil
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func runProfileUse(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile use <name>"),
			app.ExitUsage,
		)
	}
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile use <name>"), app.ExitUsage)
	}

	name := args[0]
	if err := config.Update(func(cfg *config.Config) error {
		if _, ok := cfg.Profiles[name]; !ok {
			return app.NewExitError(fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name), app.ExitConfig)
		}
		cfg.CurrentProfile = name
		return nil
	}); err != nil {
		return err
	}

	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Current profile set to %q.\n", name)
	}
	return nil
}

func runProfileCurrent(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile current"), app.ExitUsage)
	}
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

func runProfileTest(ctx context.Context, cmdCtx *Context, args []string) error {
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	if len(args) > 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile test [name]"), app.ExitUsage)
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

	if p.Endpoint == "" {
		return app.NewExitError(
			fmt.Errorf("profile %q has no endpoint configured", name),
			app.ExitConfig,
		)
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, name)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := prov.Health(ctx); err != nil {
		return app.NewExitError(
			fmt.Errorf("test connectivity: %w", err),
			app.ExitNetwork,
		)
	}

	fmt.Fprintf(cmdCtx.Writer, "Profile %q (%s): OK\n", name, p.Endpoint)
	fmt.Fprintf(cmdCtx.Writer, "  Version: %s\n", prov.Version())
	return nil
}

func runProfileRemove(_ context.Context, cmdCtx *Context, args []string) error {
	if len(args) < 1 {
		return app.NewExitError(
			fmt.Errorf("usage: nodex profile remove <name>"),
			app.ExitUsage,
		)
	}
	if len(args) > 2 || (len(args) == 2 && args[1] != "--remove-credential") {
		return app.NewExitError(fmt.Errorf("usage: nodex profile remove <name> [--remove-credential]"), app.ExitUsage)
	}

	name := args[0]
	removeCred := len(args) == 2 && args[1] == "--remove-credential"
	credentialRef := ""
	if err := config.Update(func(cfg *config.Config) error {
		p, ok := cfg.Profiles[name]
		if !ok {
			return app.NewExitError(fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name), app.ExitConfig)
		}
		credentialRef = p.CredentialRef
		delete(cfg.Profiles, name)
		if cfg.CurrentProfile == name {
			cfg.CurrentProfile = ""
			names := config.ProfileNames(cfg)
			sort.Strings(names)
			if len(names) > 0 {
				cfg.CurrentProfile = names[0]
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if removeCred {
		if err := removeCredentialForProfile(name, credentialRef); err != nil {
			return err
		}
	}
	if !cmdCtx.Opts.Quiet {
		fmt.Fprintf(cmdCtx.Writer, "Profile %q removed.\n", name)
		if removeCred {
			fmt.Fprintf(cmdCtx.Writer, "Credential for profile %q removed.\n", name)
		}
	}
	return nil
}

func removeCredentialForProfile(profileName, credentialRef string) error {
	backendName, credName := "file", profileName
	if credentialRef != "" {
		var err error
		backendName, credName, err = credentials.ParseCredentialRefStrict(credentialRef)
		if err != nil {
			return app.NewExitError(fmt.Errorf("credential_ref for profile %q: %w", profileName, err), app.ExitCredential)
		}
	}
	resolver := credentials.NewResolver("")
	backend, ok := resolver.GetBackend(backendName)
	if !ok {
		return app.NewExitError(fmt.Errorf("unknown credential backend %q", backendName), app.ExitCredential)
	}
	if err := backend.Delete(context.Background(), credName); err != nil {
		return app.NewExitError(fmt.Errorf("remove credential for profile %q: %w", profileName, err), app.ExitCredential)
	}
	return nil
}
