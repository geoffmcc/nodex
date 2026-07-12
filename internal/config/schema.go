package config

import "regexp"

const CurrentSchemaVersion = 1

// ProfileRegex validates profile names: alphanumeric start, then alphanum, underscore, or hyphen, 1-64 chars.
var ProfileRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Config is the top-level configuration structure (schema v1).
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

// DefaultConfig returns a new config with schema version 1 and empty profiles.
func DefaultConfig() *Config {
	return &Config{
		Version:  CurrentSchemaVersion,
		Profiles: make(map[string]Profile),
	}
}
