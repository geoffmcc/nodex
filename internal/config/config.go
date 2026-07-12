package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/app"
)

// Read loads the config from the default path.
func Read() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, app.NewExitError(fmt.Errorf("%w: %v", app.ErrConfigRead, err), app.ExitConfig)
	}
	return ReadFrom(path)
}

// ReadFrom loads the config from the given path.
func ReadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, app.NewExitError(
				fmt.Errorf("%w: config file not found at %s", app.ErrConfigRead, path),
				app.ExitConfig,
			)
		}
		return nil, app.NewExitError(
			fmt.Errorf("%w: %v", app.ErrConfigRead, err),
			app.ExitConfig,
		)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, app.NewExitError(
			fmt.Errorf("%w: invalid YAML: %v", app.ErrConfigInvalid, err),
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
		return app.NewExitError(fmt.Errorf("%w: %v", app.ErrConfigWrite, err), app.ExitConfig)
	}
	return WriteTo(cfg, path)
}

// WriteTo atomically saves the config to the given path (write-to-temp, rename).
func WriteTo(cfg *Config, path string) error {
	if err := Validate(cfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: marshal failed: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: temp file: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}
	tmpPath := tmp.Name()

	// Clean up temp file on error.
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return app.NewExitError(
			fmt.Errorf("%w: write temp: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return app.NewExitError(
			fmt.Errorf("%w: sync temp: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := tmp.Close(); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: close temp: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return app.NewExitError(
			fmt.Errorf("%w: rename: %v", app.ErrConfigWrite, err),
			app.ExitConfig,
		)
	}

	success = true
	return nil
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

		provider := strings.ToLower(p.Provider)
		if provider == "" {
			return app.NewExitError(
				fmt.Errorf("%w: profile %q missing provider", app.ErrProfileInvalid, name),
				app.ExitConfig,
			)
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
	return names
}

// NormalizeProvider lowercases the provider name.
func NormalizeProvider(provider string) string {
	return strings.ToLower(provider)
}
