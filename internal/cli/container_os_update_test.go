package cli

import (
	"testing"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/config"
)

func TestParseContainerOSUpdateArgs(t *testing.T) {
	if err := parseContainerOSUpdateArgs([]string{"--policy", "approved-full-upgrade"}); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	for _, args := range [][]string{
		nil,
		{"--policy"},
		{"--policy", "security-only"},
		{"--unknown", "value"},
	} {
		if err := parseContainerOSUpdateArgs(args); err == nil {
			t.Errorf("invalid args accepted: %v", args)
		}
	}
}

func TestFindPVEInventoryHost(t *testing.T) {
	cfg := &config.Config{Inventory: &config.Inventory{Hosts: map[string]config.InventoryHost{
		"pve":   {Role: config.RolePVE, PVEProfile: "production", Address: "pve.example"},
		"other": {Role: config.RoleGeneric, PVEProfile: "production", Address: "other.example"},
	}}}
	name, host, err := findPVEInventoryHost(cfg, "production")
	if err != nil {
		t.Fatalf("find host: %v", err)
	}
	if name != "pve" || host.Address != "pve.example" {
		t.Fatalf("unexpected host: %q %+v", name, host)
	}

	cfg.Inventory.Hosts["pve2"] = config.InventoryHost{Role: config.RolePVE, PVEProfile: "production"}
	if _, _, err := findPVEInventoryHost(cfg, "production"); err == nil {
		t.Fatal("ambiguous PVE hosts accepted")
	}
}

func TestInterpretContainerOSUpdate(t *testing.T) {
	result := &ansible.RunResult{
		Success: true,
		Hosts:   []ansible.HostResult{{Host: "pve", OK: 6}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {
				{Task: containerStatusTask},
				{Task: containerAPTTask},
				{Task: containerPackagesTask, StdoutLines: []string{"Listing...", "openssl/stable 3.0 amd64 [upgradable from: 2.9]"}},
				{Task: containerDpkgTask},
				{Task: containerRebootTask},
				{Task: containerRootTask, StdoutLines: []string{"Filesystem 1024-blocks Used Available Capacity Mounted on", "/dev/root 100 91 9 91% /"}},
			},
		},
	}
	state := interpretContainerOSUpdate(result, "pve")
	if !state.EvidenceComplete || len(state.PendingUpdates) != 1 || state.PendingUpdates[0] != "openssl" {
		t.Fatalf("unexpected update state: %+v", state)
	}
	if state.RootUsage != "91%" || state.DpkgIssues || state.RebootRequired {
		t.Fatalf("unexpected health state: %+v", state)
	}
}
