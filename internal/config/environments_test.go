package config

import (
	"strings"
	"testing"
)

func validEnvConfig() *Config {
	return &Config{
		Version: 2,
		Profiles: map[string]Profile{
			"production-pve": {Provider: ProviderProxmox, Endpoint: "https://pve.example.invalid:8006"},
			"production-pbs": {Provider: ProviderPBS, Endpoint: "https://pbs.example.invalid:8007"},
		},
		Environments: map[string]Environment{
			"homelab": {PVEProfile: "production-pve", PBSProfile: "production-pbs"},
		},
	}
}

func TestValidateEnvironmentsHappyPath(t *testing.T) {
	if err := Validate(validEnvConfig()); err != nil {
		t.Fatalf("valid environments config rejected: %v", err)
	}
}

func TestEnvironmentsRequireSchemaV2(t *testing.T) {
	cfg := validEnvConfig()
	cfg.Version = 1
	err := Validate(cfg)
	if err == nil {
		t.Fatal("environments on schema v1 must be rejected")
	}
	if !strings.Contains(err.Error(), "version: 2") {
		t.Errorf("error should point at the version-2 requirement, got: %v", err)
	}
}

func TestEnvironmentValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{
			name: "unknown pve profile",
			mutate: func(c *Config) {
				c.Environments["homelab"] = Environment{PVEProfile: "missing", PBSProfile: "production-pbs"}
			},
			wantErr: "unknown profile",
		},
		{
			name: "unknown pbs profile",
			mutate: func(c *Config) {
				c.Environments["homelab"] = Environment{PVEProfile: "production-pve", PBSProfile: "missing"}
			},
			wantErr: "unknown profile",
		},
		{
			name:    "pve_profile wrong provider type",
			mutate:  func(c *Config) { c.Environments["homelab"] = Environment{PVEProfile: "production-pbs"} },
			wantErr: "must reference a \"proxmox\" profile",
		},
		{
			name:    "pbs_profile wrong provider type",
			mutate:  func(c *Config) { c.Environments["homelab"] = Environment{PBSProfile: "production-pve"} },
			wantErr: "must reference a \"pbs\" profile",
		},
		{
			name:    "no profiles at all",
			mutate:  func(c *Config) { c.Environments["homelab"] = Environment{} },
			wantErr: "at least one",
		},
		{
			name:    "invalid environment name",
			mutate:  func(c *Config) { c.Environments["bad name!"] = Environment{PVEProfile: "production-pve"} },
			wantErr: "invalid environment name",
		},
		{
			name: "negative threshold",
			mutate: func(c *Config) {
				env := c.Environments["homelab"]
				env.BackupMaxAgeHours = -1
				c.Environments["homelab"] = env
			},
			wantErr: "backup_max_age_hours",
		},
		{
			name: "warn above block",
			mutate: func(c *Config) {
				env := c.Environments["homelab"]
				env.DatastoreWarnPercent = 96
				c.Environments["homelab"] = env
			},
			wantErr: "must not exceed",
		},
		{
			name: "warn above explicit block",
			mutate: func(c *Config) {
				env := c.Environments["homelab"]
				env.DatastoreWarnPercent = 90
				env.DatastoreBlockPercent = 85
				c.Environments["homelab"] = env
			},
			wantErr: "must not exceed",
		},
		{
			name: "zero vmid excluded",
			mutate: func(c *Config) {
				env := c.Environments["homelab"]
				env.ExcludeGuests = []int{0}
				c.Environments["homelab"] = env
			},
			wantErr: "positive VMIDs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validEnvConfig()
			tt.mutate(cfg)
			err := Validate(cfg)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEnvironmentPBSOnlyAllowed(t *testing.T) {
	cfg := validEnvConfig()
	cfg.Environments["backup-only"] = Environment{PBSProfile: "production-pbs"}
	if err := Validate(cfg); err != nil {
		t.Fatalf("pbs-only environment rejected: %v", err)
	}
}

func TestEnvironmentRoundTrip(t *testing.T) {
	path := writeConfigFile(t, `version: 2
profiles:
  production-pve:
    provider: proxmox
    endpoint: https://pve.example.invalid:8006
  production-pbs:
    provider: pbs
    endpoint: https://pbs.example.invalid:8007
environments:
  homelab:
    pve_profile: production-pve
    pbs_profile: production-pbs
    backup_max_age_hours: 36
    verify_max_age_days: 14
    datastore_usage_warn_percent: 75
    exclude_guests: [900]
    namespaces: ["", "prod"]
`)
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	env, ok := cfg.Environments["homelab"]
	if !ok {
		t.Fatal("environment homelab missing after read")
	}
	if env.BackupMaxAgeHours != 36 || env.VerifyMaxAgeDays != 14 || env.DatastoreWarnPercent != 75 {
		t.Errorf("thresholds not decoded: %+v", env)
	}
	if len(env.ExcludeGuests) != 1 || env.ExcludeGuests[0] != 900 {
		t.Errorf("exclude_guests not decoded: %+v", env.ExcludeGuests)
	}
	if len(env.Namespaces) != 2 || env.Namespaces[1] != "prod" {
		t.Errorf("namespaces not decoded: %+v", env.Namespaces)
	}
	if err := WriteTo(cfg, path); err != nil {
		t.Fatalf("write: %v", err)
	}
	reread, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(reread.Environments) != 1 {
		t.Errorf("environments lost on round-trip: %+v", reread.Environments)
	}
}

func TestEnvironmentNamesSorted(t *testing.T) {
	cfg := validEnvConfig()
	cfg.Environments["alpha"] = Environment{PBSProfile: "production-pbs"}
	got := EnvironmentNames(cfg)
	if len(got) != 2 || got[0] != "alpha" || got[1] != "homelab" {
		t.Errorf("EnvironmentNames = %v", got)
	}
}
