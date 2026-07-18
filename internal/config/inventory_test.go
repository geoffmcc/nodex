package config

import (
	"os"
	"strings"
	"testing"
)

func validInventoryConfig() *Config {
	cfg := validEnvConfig()
	cfg.Inventory = &Inventory{
		Hosts: map[string]InventoryHost{
			"pve-primary": {
				Address:          "pve.example.invalid",
				Role:             RolePVE,
				Environment:      "homelab",
				PVEProfile:       "production-pve",
				SSHUser:          "automation",
				SSHPort:          22,
				SSHKeyFile:       "~/.ssh/nodex_automation",
				MaintenanceGroup: "hypervisors",
				Criticality:      CriticalityCritical,
				BackupRequired:   true,
			},
			"dns-primary": {
				Address:     "10.0.0.53",
				Role:        RoleDNS,
				SSHUser:     "automation",
				Criticality: CriticalityCritical,
			},
		},
	}
	return cfg
}

func TestValidateInventoryHappyPath(t *testing.T) {
	if err := Validate(validInventoryConfig()); err != nil {
		t.Fatalf("valid inventory rejected: %v", err)
	}
}

func TestInventoryRequiresSchemaV2(t *testing.T) {
	cfg := validInventoryConfig()
	cfg.Version = 1
	cfg.Environments = nil
	for name, h := range cfg.Inventory.Hosts {
		h.Environment = ""
		cfg.Inventory.Hosts[name] = h
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("inventory on schema v1 must be rejected")
	}
	if !strings.Contains(err.Error(), "version: 2") {
		t.Errorf("error should point at the version-2 requirement, got: %v", err)
	}
}

func TestInventoryValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(h *InventoryHost)
		wantErr string
	}{
		{"missing address", func(h *InventoryHost) { h.Address = "" }, "address is required"},
		{"scheme in address", func(h *InventoryHost) { h.Address = "https://pve.example.invalid" }, "invalid address"},
		{"userinfo in address", func(h *InventoryHost) { h.Address = "root@pve.example.invalid" }, "invalid address"},
		{"space in address", func(h *InventoryHost) { h.Address = "bad host" }, "invalid address"},
		{"missing role", func(h *InventoryHost) { h.Role = "" }, "role is required"},
		{"bad role shape", func(h *InventoryHost) { h.Role = "Bad Role!" }, "invalid role"},
		{"missing ssh user", func(h *InventoryHost) { h.SSHUser = "" }, "ssh_user is required"},
		{"ssh user with colon", func(h *InventoryHost) { h.SSHUser = "user:pass" }, "invalid ssh_user"},
		{"ssh user with at", func(h *InventoryHost) { h.SSHUser = "user@host" }, "invalid ssh_user"},
		{"ssh user with quote", func(h *InventoryHost) { h.SSHUser = `a"b` }, "invalid ssh_user"},
		{"port too high", func(h *InventoryHost) { h.SSHPort = 70000 }, "ssh_port"},
		{"negative port", func(h *InventoryHost) { h.SSHPort = -1 }, "ssh_port"},
		{"bad criticality", func(h *InventoryHost) { h.Criticality = "very-important" }, "criticality"},
		{"bad maintenance group", func(h *InventoryHost) { h.MaintenanceGroup = "bad group!" }, "maintenance_group"},
		{"unknown environment", func(h *InventoryHost) { h.Environment = "nonexistent" }, "unknown environment"},
		{"unknown pve profile", func(h *InventoryHost) { h.PVEProfile = "missing" }, "unknown profile"},
		{"pbs profile wrong type", func(h *InventoryHost) { h.PBSProfile = "production-pve" }, "must reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validInventoryConfig()
			h := cfg.Inventory.Hosts["pve-primary"]
			tt.mutate(&h)
			cfg.Inventory.Hosts["pve-primary"] = h
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

func TestInventoryHostNameValidation(t *testing.T) {
	cfg := validInventoryConfig()
	cfg.Inventory.Hosts["bad name!"] = InventoryHost{
		Address: "x.example.invalid", Role: RoleGeneric, SSHUser: "automation",
	}
	if err := Validate(cfg); err == nil {
		t.Fatal("invalid host name must be rejected")
	}
}

// TestInventoryHasNoSecretFields pins the schema contract: the inventory
// host struct must never grow fields for key material or passwords.
func TestInventoryHasNoSecretFields(t *testing.T) {
	forbidden := []string{"password", "secret", "private_key", "key_data", "vault", "sudo_pass", "passphrase", "token"}
	// Reflection-free check via the YAML round trip of a fully populated host.
	cfg := validInventoryConfig()
	path := writeConfigFile(t, "version: 2\nprofiles: {}\n")
	if err := WriteTo(cfg, path); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- test reads its own temp file
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, f := range forbidden {
		if strings.Contains(lower, f+":") {
			t.Errorf("serialized inventory contains forbidden field %q", f)
		}
	}
}

func TestInventoryRoundTrip(t *testing.T) {
	path := writeConfigFile(t, `version: 2
profiles:
  production-pve:
    provider: proxmox
    endpoint: https://pve.example.invalid:8006
inventory:
  hosts:
    dns-primary:
      address: dns.example.invalid
      role: dns
      ssh_user: automation
      ssh_port: 2222
      ssh_key_file: ~/.ssh/nodex_automation
      known_hosts_file: ~/.ssh/known_hosts_nodex
      maintenance_group: infrastructure
      criticality: critical
      backup_required: true
`)
	cfg, err := ReadFrom(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	h, ok := cfg.Inventory.Hosts["dns-primary"]
	if !ok {
		t.Fatal("host missing after read")
	}
	if h.SSHPort != 2222 || h.KnownHostsFile != "~/.ssh/known_hosts_nodex" || !h.BackupRequired {
		t.Errorf("host not decoded correctly: %+v", h)
	}
	if h.AutomaticReboot {
		t.Error("automatic_reboot must default to false")
	}
	names := InventoryHostNames(cfg)
	if len(names) != 1 || names[0] != "dns-primary" {
		t.Errorf("InventoryHostNames = %v", names)
	}
}
