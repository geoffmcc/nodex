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
	Version        int                                 `yaml:"version"`
	CurrentProfile string                              `yaml:"current_profile"`
	Profiles       map[string]Profile                  `yaml:"profiles"`
	Environments   map[string]Environment              `yaml:"environments,omitempty"`
	Inventory      *Inventory                          `yaml:"inventory,omitempty"`
	Monitoring     *Monitoring                         `yaml:"monitoring,omitempty"`
	Certifications map[string]CertificationEnvironment `yaml:"certifications,omitempty"`
}

// CertificationEnvironment is an explicit authorization binding for the
// opt-in certification suites. Names and profile prefixes are not security
// boundaries; every mutation must match this record exactly.
type CertificationEnvironment struct {
	Profile             string   `yaml:"profile"`
	Endpoint            string   `yaml:"endpoint"`
	Provider            string   `yaml:"provider"`
	Nodes               []string `yaml:"nodes"`
	Storage             []string `yaml:"storage"`
	VMIDMin             int      `yaml:"vmid_min"`
	VMIDMax             int      `yaml:"vmid_max"`
	Suites              []string `yaml:"suites"`
	AllowMutations      bool     `yaml:"allow_mutations"`
	MaxResources        int      `yaml:"max_resources"`
	ExpiresAt           int64    `yaml:"expires_at"`
	ExpectedFingerprint string   `yaml:"expected_fingerprint,omitempty"`
	TrustedCAIdentity   string   `yaml:"trusted_ca_identity,omitempty"`
}

// Monitoring contains only explicitly configured one-shot checks. Nodex never
// discovers targets or opens connections that are not represented here.
type Monitoring struct {
	Targets     map[string]MonitorTarget `yaml:"targets"`
	Concurrency int                      `yaml:"concurrency,omitempty"`
	Timeout     int                      `yaml:"timeout_seconds,omitempty"`
}

type MonitorTarget struct {
	Type           string `yaml:"type" json:"type"`
	Address        string `yaml:"address" json:"address"`
	Environment    string `yaml:"environment,omitempty" json:"environment,omitempty"`
	Timeout        int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	Resolver       string `yaml:"resolver,omitempty" json:"resolver,omitempty"`
	ExpiresIn      int    `yaml:"expiry_warning_days,omitempty" json:"expiry_warning_days,omitempty"`
	CAFile         string `yaml:"ca_file,omitempty" json:"ca_file,omitempty"`
	ExpectedStatus int    `yaml:"expected_status,omitempty" json:"expected_status,omitempty"`
	Service        string `yaml:"service,omitempty" json:"service,omitempty"`
	Application    string `yaml:"application,omitempty" json:"application,omitempty"`
}

// Inventory declares the SSH-manageable Linux hosts. Hosts must be enrolled
// explicitly — Proxmox discovery never implies SSH manageability. The
// inventory stores no secrets: SSH authentication uses the agent or the
// referenced key file, never embedded key material or passwords.
type Inventory struct {
	Hosts map[string]InventoryHost `yaml:"hosts"`
}

// InventoryHost is one explicitly enrolled host. JSON tags cover CLI output
// (e.g. `maintenance inventory --output json`); the config file itself is
// YAML.
type InventoryHost struct {
	Address        string `yaml:"address" json:"address"`
	Role           string `yaml:"role" json:"role"`
	Environment    string `yaml:"environment,omitempty" json:"environment,omitempty"`
	PVEProfile     string `yaml:"pve_profile,omitempty" json:"pve_profile,omitempty"`
	PBSProfile     string `yaml:"pbs_profile,omitempty" json:"pbs_profile,omitempty"`
	SSHUser        string `yaml:"ssh_user" json:"ssh_user"`
	SSHPort        int    `yaml:"ssh_port,omitempty" json:"ssh_port,omitempty"`
	SSHKeyFile     string `yaml:"ssh_key_file,omitempty" json:"ssh_key_file,omitempty"`
	KnownHostsFile string `yaml:"known_hosts_file,omitempty" json:"known_hosts_file,omitempty"`

	MaintenanceGroup string `yaml:"maintenance_group,omitempty" json:"maintenance_group,omitempty"`
	Criticality      string `yaml:"criticality,omitempty" json:"criticality,omitempty"`
	BackupRequired   bool   `yaml:"backup_required,omitempty" json:"backup_required,omitempty"`

	// AutomaticReboot must be explicitly enabled per host; the zero value
	// (false) is the default for every role.
	AutomaticReboot bool `yaml:"automatic_reboot,omitempty" json:"automatic_reboot,omitempty"`
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
