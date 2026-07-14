package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/credentials"
)

// Read loads the config from the default path.
func Read() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("%w: %w", app.ErrConfigRead, err), app.ExitConfig)
	}
	return ReadFrom(path)
}

// ReadFrom loads the config from the given path.
func ReadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config paths are resolved by Nodex path helpers or explicit test inputs.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, app.NewExitError(
				fmt.Errorf("%w: config file not found at %s", app.ErrConfigRead, path),
				app.ExitConfig,
			)
		}
		return nil, app.NewExitError(
			fmt.Errorf("%w: %w", app.ErrConfigRead, err),
			app.ExitConfig,
		)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("%w: invalid YAML: %w", app.ErrConfigInvalid, err),
			app.ExitConfig,
		)
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Write atomically saves the config to the default path.
func Write(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return app.NewExitError(fmt.Errorf("%w: %w", app.ErrConfigWrite, err), app.ExitConfig)
	}
	lock, err := Lock(path)
	if err != nil {
		return app.NewExitError(fmt.Errorf("%w: lock: %w", app.ErrConfigWrite, err), app.ExitConfig)
	}
	defer func() { _ = Unlock(lock) }()
	return writeToUnlocked(cfg, path)
}

// WriteTo atomically saves the config to the given path (write-to-temp, rename).
func WriteTo(cfg *Config, path string) error {
	lock, err := Lock(path)
	if err != nil {
		return app.NewExitError(fmt.Errorf("%w: lock: %w", app.ErrConfigWrite, err), app.ExitConfig)
	}
	defer func() { _ = Unlock(lock) }()
	return writeToUnlocked(cfg, path)
}

func writeToUnlocked(cfg *Config, path string) error {
	if err := Validate(cfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: marshal failed: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: temp file: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}
	tmpPath := tmp.Name()

	// Ensure config file is not world-readable (defense-in-depth; Go's
	// os.CreateTemp uses 0600 on Unix, but this makes the intent explicit
	// and protects against platforms where the default differs).
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return app.NewExitError(
			fmt.Errorf("%w: secure temp file: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	// Clean up temp file on error.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return app.NewExitError(
			fmt.Errorf("%w: write temp: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return app.NewExitError(
			fmt.Errorf("%w: sync temp: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := tmp.Close(); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: close temp: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: rename: %w", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	success = true
	return nil
}

// Update loads, mutates, validates, and writes the default config while holding
// the config lock for the complete read-modify-write transaction.
func Update(mutator func(*Config) error) error {
	path, err := ConfigPath()
	if err != nil {
		return app.NewExitError(fmt.Errorf("%w: %w", app.ErrConfigRead, err), app.ExitConfig)
	}
	lock, err := Lock(path)
	if err != nil {
		return app.NewExitError(fmt.Errorf("%w: lock: %w", app.ErrConfigWrite, err), app.ExitConfig)
	}
	defer func() { _ = Unlock(lock) }()
	cfg, err := ReadFrom(path)
	if err != nil {
		return err
	}
	if err := mutator(cfg); err != nil {
		return err
	}
	return writeToUnlocked(cfg, path)
}

// Validate checks a config for structural and semantic correctness.
func Validate(cfg *Config) error {
	if cfg == nil {
		return app.NewExitError(
			fmt.Errorf("%w: config is nil", app.ErrConfigInvalid),
			app.ExitConfig,
		)
	}

	if cfg.Version != CurrentSchemaVersion {
		return app.NewExitError(
			fmt.Errorf("%w: unsupported schema version %d (expected %d)",
				app.ErrConfigInvalid, cfg.Version, CurrentSchemaVersion),
			app.ExitConfig,
		)
	}

	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]Profile)
	}

	for name, p := range cfg.Profiles {
		if !ProfileRegex.MatchString(name) {
			return app.NewExitError(
				fmt.Errorf("%w: invalid profile name %q (must match %s)",
					app.ErrProfileInvalid, name, ProfileRegex.String()),
				app.ExitConfig,
			)
		}

		provider := strings.TrimSpace(strings.ToLower(p.Provider))
		if provider == "" {
			return app.NewExitError(
				fmt.Errorf("%w: profile %q missing provider", app.ErrProfileInvalid, name),
				app.ExitConfig,
			)
		}
		if p.Provider != provider {
			cfg.Profiles[name] = Profile{Provider: provider, Endpoint: p.Endpoint, CredentialRef: p.CredentialRef, CAFile: p.CAFile}
		}
		if p.Endpoint != "" {
			if err := ValidateEndpoint(p.Endpoint); err != nil {
				return app.NewExitError(
					fmt.Errorf("%w: profile %q endpoint: %w", app.ErrProfileInvalid, name, err),
					app.ExitConfig,
				)
			}
		}
		if p.CredentialRef != "" {
			if _, _, err := credentials.ParseCredentialRefStrict(p.CredentialRef); err != nil {
				return app.NewExitError(
					fmt.Errorf("%w: profile %q credential_ref: %w", app.ErrProfileInvalid, name, err),
					app.ExitConfig,
				)
			}
		}
	}

	if cfg.CurrentProfile != "" {
		if _, ok := cfg.Profiles[cfg.CurrentProfile]; !ok {
			return app.NewExitError(
				fmt.Errorf("%w: current_profile %q not found in profiles",
					app.ErrProfileNotFound, cfg.CurrentProfile),
				app.ExitConfig,
			)
		}
	}

	return nil
}

// ProfileNames returns the sorted list of profile names.
func ProfileNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NormalizeProvider lowercases the provider name.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ValidateEndpoint enforces Nodex's default HTTPS endpoint policy.
func ValidateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("malformed URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("must use https scheme")
	}
	if u.Host == "" || u.User != nil {
		return fmt.Errorf("must include host and must not include user info")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must not include query string or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("must not include a path")
	}
	return nil
}
