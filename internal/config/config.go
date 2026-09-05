package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/credentials"
)

var sha256FingerprintRegex = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

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

	if cfg.Version > CurrentSchemaVersion {
		return app.NewExitError(
			fmt.Errorf("%w: schema version %d is newer than this nodex supports (maximum %d); upgrade nodex",
				app.ErrConfigInvalid, cfg.Version, CurrentSchemaVersion),
			app.ExitConfig,
		)
	}
	if cfg.Version < MinSupportedSchemaVersion {
		return app.NewExitError(
			fmt.Errorf("%w: unsupported schema version %d (supported: %d-%d)",
				app.ErrConfigInvalid, cfg.Version, MinSupportedSchemaVersion, CurrentSchemaVersion),
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

		provider := NormalizeProvider(p.Provider)
		if provider == "" {
			return app.NewExitError(
				fmt.Errorf("%w: profile %q missing provider", app.ErrProfileInvalid, name),
				app.ExitConfig,
			)
		}
		// Shape-only validation: unknown provider names stay loadable so a
		// config written by a newer Nodex does not brick every profile here;
		// provider existence is enforced when a command uses the profile.
		if !ProviderRegex.MatchString(provider) {
			return app.NewExitError(
				fmt.Errorf("%w: profile %q invalid provider name %q (must match %s)",
					app.ErrProfileInvalid, name, provider, ProviderRegex.String()),
				app.ExitConfig,
			)
		}
		if p.Provider != provider {
			p.Provider = provider
			cfg.Profiles[name] = p
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

	if err := validateEnvironments(cfg); err != nil {
		return err
	}

	if err := validateInventory(cfg); err != nil {
		return err
	}
	if err := validateMonitoring(cfg); err != nil {
		return err
	}
	if err := validateCertifications(cfg); err != nil {
		return err
	}

	return nil
}

func validateCertifications(cfg *Config) error {
	for name, env := range cfg.Certifications {
		fail := func(format string, args ...any) error {
			return app.NewExitError(fmt.Errorf("%w: certification environment %q: %s", app.ErrConfigInvalid, name, fmt.Sprintf(format, args...)), app.ExitConfig)
		}
		if !ProfileRegex.MatchString(name) || env.Profile == "" || !ProfileRegex.MatchString(env.Profile) {
			return fail("invalid environment or profile name")
		}
		if env.Profile != "nodex-test-admin" {
			return fail("profile must be nodex-test-admin")
		}
		p, ok := cfg.Profiles[env.Profile]
		if !ok {
			return fail("profile %q is not configured", env.Profile)
		}
		if err := ValidateEndpoint(env.Endpoint); err != nil {
			return fail("endpoint: %v", err)
		}
		if env.Endpoint != p.Endpoint {
			return fail("endpoint does not match profile")
		}
		if env.Provider == "" {
			return fail("provider is required")
		}
		if !sha256FingerprintRegex.MatchString(env.ExpectedFingerprint) {
			return fail("expected_fingerprint must be a SHA-256 fingerprint")
		}
		if !sha256FingerprintRegex.MatchString(env.TrustedCAIdentity) {
			return fail("trusted_ca_identity must be a SHA-256 fingerprint")
		}
		if NormalizeProvider(env.Provider) != NormalizeProvider(p.Provider) {
			return fail("provider does not match profile")
		}
		if env.VMIDMin <= 0 || env.VMIDMax < env.VMIDMin {
			return fail("invalid VMID range")
		}
		if env.MaxResources <= 0 || env.MaxResources > 100 {
			return fail("max_resources must be between 1 and 100")
		}
		if env.ExpiresAt <= 0 || env.ExpiresAt <= time.Now().Unix() {
			return fail("authorization is expired")
		}
		seen := map[string]bool{}
		for _, suite := range env.Suites {
			if suite != "readonly" && suite != "disposable-mutations" {
				return fail("unsupported suite %q", suite)
			}
			if seen[suite] {
				return fail("duplicate suite %q", suite)
			}
			seen[suite] = true
		}
		if !env.AllowMutations {
			for _, suite := range env.Suites {
				if suite == "disposable-mutations" {
					return fail("mutation suite requires allow_mutations")
				}
			}
		}
		for _, node := range env.Nodes {
			if !ProfileRegex.MatchString(node) {
				return fail("invalid node name")
			}
		}
		for _, storage := range env.Storage {
			if !ProfileRegex.MatchString(storage) {
				return fail("invalid storage name")
			}
		}
	}
	return nil
}

func validateMonitoring(cfg *Config) error {
	if cfg.Monitoring == nil || len(cfg.Monitoring.Targets) == 0 {
		return nil
	}
	if cfg.Version < 2 {
		return app.NewExitError(fmt.Errorf("%w: the monitoring section requires schema version 2 (set \"version: 2\")", app.ErrConfigInvalid), app.ExitConfig)
	}
	if cfg.Monitoring.Concurrency < 0 || cfg.Monitoring.Concurrency > 32 {
		return app.NewExitError(fmt.Errorf("%w: monitoring concurrency must be between 0 and 32", app.ErrConfigInvalid), app.ExitConfig)
	}
	if cfg.Monitoring.Timeout < 0 || cfg.Monitoring.Timeout > 300 {
		return app.NewExitError(fmt.Errorf("%w: monitoring timeout must be between 0 and 300 seconds", app.ErrConfigInvalid), app.ExitConfig)
	}
	for name, target := range cfg.Monitoring.Targets {
		if !ProfileRegex.MatchString(name) {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q has invalid name", app.ErrConfigInvalid, name), app.ExitConfig)
		}
		switch target.Type {
		case "http", "https", "tcp", "tls", "dns", "pve-api", "pbs-api", "pve-tasks", "pbs-tasks", "datastore", "backup-age", "backup-verification", "backup-coverage", "service", "application":
		default:
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q has unsupported type %q", app.ErrConfigInvalid, name, target.Type), app.ExitConfig)
		}
		if strings.TrimSpace(target.Address) == "" {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q address is required", app.ErrConfigInvalid, name), app.ExitConfig)
		}
		if target.Timeout < 0 || target.Timeout > 300 {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q timeout must be between 0 and 300 seconds", app.ErrConfigInvalid, name), app.ExitConfig)
		}
		if target.Type == "http" || target.Type == "https" {
			u, err := url.Parse(target.Address)
			if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || (target.Type == "https" && u.Scheme != "https") || (target.Type == "http" && u.Scheme != "http") {
				return app.NewExitError(fmt.Errorf("%w: monitoring target %q must be a credential-free %s URL", app.ErrConfigInvalid, name, target.Type), app.ExitConfig)
			}
		}
		if target.Type == "tls" {
			if _, _, err := net.SplitHostPort(target.Address); err != nil {
				return app.NewExitError(fmt.Errorf("%w: monitoring target %q must be host:port", app.ErrConfigInvalid, name), app.ExitConfig)
			}
		}
		if target.Type == "dns" && target.Resolver == "" {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q requires an explicit resolver", app.ErrConfigInvalid, name), app.ExitConfig)
		}
		if target.Type == "dns" {
			if _, _, err := net.SplitHostPort(target.Resolver); err != nil {
				return app.NewExitError(fmt.Errorf("%w: monitoring target %q resolver must be host:port", app.ErrConfigInvalid, name), app.ExitConfig)
			}
		}
		if target.Type != "http" && target.Type != "https" && (strings.HasSuffix(target.Type, "-api") || strings.HasSuffix(target.Type, "-tasks") || target.Type == "application") {
			u, err := url.Parse(target.Address)
			if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" {
				return app.NewExitError(fmt.Errorf("%w: monitoring target %q must be a credential-free HTTPS URL", app.ErrConfigInvalid, name), app.ExitConfig)
			}
		}
		if target.Type == "tls" && target.ExpiresIn < 0 {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q expiry warning must be non-negative", app.ErrConfigInvalid, name), app.ExitConfig)
		}
		if target.ExpectedStatus < 0 || target.ExpectedStatus > 599 {
			return app.NewExitError(fmt.Errorf("%w: monitoring target %q expected status is invalid", app.ErrConfigInvalid, name), app.ExitConfig)
		}
	}
	return nil
}

// hostAddressRegex matches hostnames and IP literals (no scheme, no port,
// no userinfo).
var hostAddressRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.:-]{0,252}[a-zA-Z0-9])?$`)

// validateInventory checks the schema-version-2-only inventory section.
func validateInventory(cfg *Config) error {
	if cfg.Inventory == nil || len(cfg.Inventory.Hosts) == 0 {
		return nil
	}
	if cfg.Version < 2 {
		return app.NewExitError(
			fmt.Errorf("%w: the inventory section requires schema version 2 (set \"version: 2\")",
				app.ErrConfigInvalid),
			app.ExitConfig,
		)
	}
	for name, h := range cfg.Inventory.Hosts {
		fail := func(format string, args ...any) error {
			detail := fmt.Sprintf(format, args...)
			return app.NewExitError(
				fmt.Errorf("%w: inventory host %q: %s", app.ErrConfigInvalid, name, detail),
				app.ExitConfig,
			)
		}
		if !ProfileRegex.MatchString(name) {
			return fail("invalid host name (must match %s)", ProfileRegex.String())
		}
		if h.Address == "" {
			return fail("address is required")
		}
		if !hostAddressRegex.MatchString(h.Address) {
			return fail("invalid address %q (hostname or IP, no scheme, port, or userinfo)", h.Address)
		}
		if h.Role == "" {
			return fail("role is required (e.g. %s, %s, %s, %s)", RolePVE, RolePBS, RoleDNS, RoleGeneric)
		}
		if !ProviderRegex.MatchString(h.Role) {
			return fail("invalid role %q", h.Role)
		}
		if h.SSHUser == "" {
			return fail("ssh_user is required")
		}
		if strings.ContainsAny(h.SSHUser, " \t:@/\\'\"") {
			return fail("invalid ssh_user %q", h.SSHUser)
		}
		if h.SSHPort < 0 || h.SSHPort > 65535 {
			return fail("ssh_port must be between 1 and 65535")
		}
		if h.Criticality != "" && h.Criticality != CriticalityCritical && h.Criticality != CriticalityStandard {
			return fail("criticality must be %q or %q", CriticalityCritical, CriticalityStandard)
		}
		if h.MaintenanceGroup != "" && !ProfileRegex.MatchString(h.MaintenanceGroup) {
			return fail("invalid maintenance_group %q", h.MaintenanceGroup)
		}
		if h.Environment != "" {
			if _, ok := cfg.Environments[h.Environment]; !ok {
				return fail("references unknown environment %q", h.Environment)
			}
		}
		owner := fmt.Sprintf("inventory host %q", name)
		if err := validateProfileRef(cfg, owner, "pve_profile", h.PVEProfile, ProviderProxmox); err != nil {
			return err
		}
		if err := validateProfileRef(cfg, owner, "pbs_profile", h.PBSProfile, ProviderPBS); err != nil {
			return err
		}
	}
	return nil
}

// InventoryHostNames returns the sorted list of inventory host names.
func InventoryHostNames(cfg *Config) []string {
	if cfg.Inventory == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Inventory.Hosts))
	for name := range cfg.Inventory.Hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validateEnvironments checks the schema-version-2-only environments section.
func validateEnvironments(cfg *Config) error {
	if len(cfg.Environments) == 0 {
		return nil
	}
	if cfg.Version < 2 {
		return app.NewExitError(
			fmt.Errorf("%w: the environments section requires schema version 2 (set \"version: 2\")",
				app.ErrConfigInvalid),
			app.ExitConfig,
		)
	}
	for name, env := range cfg.Environments {
		if !ProfileRegex.MatchString(name) {
			return app.NewExitError(
				fmt.Errorf("%w: invalid environment name %q (must match %s)",
					app.ErrConfigInvalid, name, ProfileRegex.String()),
				app.ExitConfig,
			)
		}
		if env.PVEProfile == "" && env.PBSProfile == "" {
			return app.NewExitError(
				fmt.Errorf("%w: environment %q must reference at least one of pve_profile or pbs_profile",
					app.ErrConfigInvalid, name),
				app.ExitConfig,
			)
		}
		owner := fmt.Sprintf("environment %q", name)
		if err := validateProfileRef(cfg, owner, "pve_profile", env.PVEProfile, ProviderProxmox); err != nil {
			return err
		}
		if err := validateProfileRef(cfg, owner, "pbs_profile", env.PBSProfile, ProviderPBS); err != nil {
			return err
		}
		for _, field := range []struct {
			label string
			value int
			max   int
		}{
			{"backup_max_age_hours", env.BackupMaxAgeHours, 24 * 365},
			{"verify_max_age_days", env.VerifyMaxAgeDays, 365},
			{"datastore_usage_warn_percent", env.DatastoreWarnPercent, 100},
			{"datastore_usage_block_percent", env.DatastoreBlockPercent, 100},
		} {
			if field.value < 0 || field.value > field.max {
				return app.NewExitError(
					fmt.Errorf("%w: environment %q %s must be between 0 and %d",
						app.ErrConfigInvalid, name, field.label, field.max),
					app.ExitConfig,
				)
			}
		}
		warn, block := env.DatastoreWarnPercent, env.DatastoreBlockPercent
		if warn == 0 {
			warn = DefaultDatastoreWarnPercent
		}
		if block == 0 {
			block = DefaultDatastoreBlockPercent
		}
		if warn > block {
			return app.NewExitError(
				fmt.Errorf("%w: environment %q datastore_usage_warn_percent (%d) must not exceed datastore_usage_block_percent (%d)",
					app.ErrConfigInvalid, name, warn, block),
				app.ExitConfig,
			)
		}
		for _, vmid := range env.ExcludeGuests {
			if vmid <= 0 {
				return app.NewExitError(
					fmt.Errorf("%w: environment %q exclude_guests entries must be positive VMIDs",
						app.ErrConfigInvalid, name),
					app.ExitConfig,
				)
			}
		}
	}
	return nil
}

// validateProfileRef checks that a referenced profile exists and uses the
// expected provider. The owner string names the referencing section entry
// (e.g. `environment "homelab"` or `inventory host "pve-primary"`).
func validateProfileRef(cfg *Config, owner, field, profileName, wantProvider string) error {
	if profileName == "" {
		return nil
	}
	p, ok := cfg.Profiles[profileName]
	if !ok {
		return app.NewExitError(
			fmt.Errorf("%w: %s %s references unknown profile %q",
				app.ErrConfigInvalid, owner, field, profileName),
			app.ExitConfig,
		)
	}
	// Only known provider names are type-checked; unknown providers (e.g.
	// from a newer Nodex) stay loadable and fail at use instead.
	provider := NormalizeProvider(p.Provider)
	if IsKnownProvider(provider) && provider != wantProvider {
		return app.NewExitError(
			fmt.Errorf("%w: %s %s must reference a %q profile, but %q uses provider %q",
				app.ErrConfigInvalid, owner, field, wantProvider, profileName, p.Provider),
			app.ExitConfig,
		)
	}
	return nil
}

// EnvironmentNames returns the sorted list of environment names.
func EnvironmentNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Environments))
	for name := range cfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
