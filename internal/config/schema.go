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
// The environments and inventory sections are schema-version-2-only.
type Config struct {
	Version        int                    `yaml:"version"`
	CurrentProfile string                 `yaml:"current_profile"`
	Profiles       map[string]Profile     `yaml:"profiles"`
	Environments   map[string]Environment `yaml:"environments,omitempty"`
	Inventory      *Inventory             `yaml:"inventory,omitempty"`
}

// Inventory declares the SSH-manageable Linux hosts. Hosts must be enrolled
// explicitly — Proxmox discovery never implies SSH manageability. The
// inventory stores no secrets: SSH authentication uses the agent or the
// referenced key file, never embedded key material or passwords.
type Inventory struct {
	Hosts map[string]InventoryHost `yaml:"hosts"`
}

// InventoryHost is one explicitly enrolled host.
type InventoryHost struct {
	Address        string `yaml:"address"`
	Role           string `yaml:"role"`
	Environment    string `yaml:"environment,omitempty"`
	PVEProfile     string `yaml:"pve_profile,omitempty"`
	PBSProfile     string `yaml:"pbs_profile,omitempty"`
	SSHUser        string `yaml:"ssh_user"`
	SSHPort        int    `yaml:"ssh_port,omitempty"`
	SSHKeyFile     string `yaml:"ssh_key_file,omitempty"`
	KnownHostsFile string `yaml:"known_hosts_file,omitempty"`

	MaintenanceGroup string `yaml:"maintenance_group,omitempty"`
	Criticality      string `yaml:"criticality,omitempty"`
	BackupRequired   bool   `yaml:"backup_required,omitempty"`

	// AutomaticReboot must be explicitly enabled per host; the zero value
	// (false) is the default for every role.
	AutomaticReboot bool `yaml:"automatic_reboot,omitempty"`
}

// Known host roles. Role is informational plus safety-relevant: pve, pbs,
// and dns hosts get extra protection in maintenance sequencing.
const (
	RolePVE     = "pve"
	RolePBS     = "pbs"
	RoleDNS     = "dns"
	RoleGeneric = "generic"
)

// Criticality levels.
const (
	CriticalityCritical = "critical"
	CriticalityStandard = "standard"
)

// Environment groups a Proxmox VE profile and a Proxmox Backup Server
// profile for unified health and backup-health evaluation. Threshold fields
// use zero to mean "use the default"; defaults are exposed as constants.
type Environment struct {
	PVEProfile string `yaml:"pve_profile"`
	PBSProfile string `yaml:"pbs_profile"`

	// BackupMaxAgeHours is the maximum age of a protected guest's newest
	// backup before coverage degrades to warning. Default 26 (daily backups
	// plus slack).
	BackupMaxAgeHours int `yaml:"backup_max_age_hours,omitempty"`

	// VerifyMaxAgeDays is the maximum age of a snapshot before its missing
	// verification degrades to warning. Default 8.
	VerifyMaxAgeDays int `yaml:"verify_max_age_days,omitempty"`

	// DatastoreWarnPercent and DatastoreBlockPercent are datastore usage
	// thresholds. Defaults 80 and 95.
	DatastoreWarnPercent  int `yaml:"datastore_usage_warn_percent,omitempty"`
	DatastoreBlockPercent int `yaml:"datastore_usage_block_percent,omitempty"`

	// Namespaces are the PBS namespaces searched for guest backups. Empty
	// means the root namespace only.
	Namespaces []string `yaml:"namespaces,omitempty"`

	// ExcludeGuests lists VMIDs exempt from backup-coverage checks. All
	// other PVE guests are treated as protected.
	ExcludeGuests []int `yaml:"exclude_guests,omitempty"`
}

// Environment threshold defaults.
const (
	DefaultBackupMaxAgeHours     = 26
	DefaultVerifyMaxAgeDays      = 8
	DefaultDatastoreWarnPercent  = 80
	DefaultDatastoreBlockPercent = 95
)

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
