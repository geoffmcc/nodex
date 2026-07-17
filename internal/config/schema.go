package config

import "regexp"

// CurrentSchemaVersion is the schema version written for new configurations.
// MinSupportedSchemaVersion is the oldest schema version the loader accepts.
// Version 1 files load with unchanged semantics and keep their version on
// read-modify-write; Nodex never silently rewrites a config to a newer
// schema version (see docs/adr/0001-fleet-operations-architecture.md).
const (
	CurrentSchemaVersion      = 2
	MinSupportedSchemaVersion = 1
)

// Known provider names. Provider naming is stable: "proxmox" is Proxmox VE;
// "pbs" is Proxmox Backup Server.
const (
	ProviderProxmox = "proxmox"
	ProviderPBS     = "pbs"
)

// KnownProviders lists the provider names Nodex understands, in display order.
func KnownProviders() []string {
	return []string{ProviderProxmox, ProviderPBS}
}

// IsKnownProvider reports whether the (normalized) provider name is one Nodex
// understands. Config files may contain unknown provider names (they fail
// only when a command uses that profile); CLI entry points reject them.
func IsKnownProvider(provider string) bool {
	switch provider {
	case ProviderProxmox, ProviderPBS:
		return true
	}
	return false
}

// ProfileRegex validates profile names: alphanumeric start, then alphanum, underscore, or hyphen, 1-64 chars.
var ProfileRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ProviderRegex validates the shape of provider names in config files:
// lowercase alphanumeric start, then lowercase alphanum, underscore, or
// hyphen, 1-32 chars. Shape-only so a config written by a newer Nodex with an
// additional provider type still loads here; existence is checked at use.
var ProviderRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Config is the top-level configuration structure (schema versions 1-2).
type Config struct {
	Version        int                `yaml:"version"`
	CurrentProfile string             `yaml:"current_profile"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

// Profile holds connection details for a single provider target.
type Profile struct {
	Provider      string `yaml:"provider"`
	Endpoint      string `yaml:"endpoint"`
	CredentialRef string `yaml:"credential_ref"`
	CAFile        string `yaml:"ca_file,omitempty"`
}

// DefaultConfig returns a new config with the current schema version and empty profiles.
func DefaultConfig() *Config {
	return &Config{
		Version:  CurrentSchemaVersion,
		Profiles: make(map[string]Profile),
	}
}
