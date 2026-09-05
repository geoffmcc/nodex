package cli

import (
	"strings"
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
		"pve":   {Role: config.RolePVE, PVENode: "node-a", PVEProfile: "production", Address: "pve.example"},
		"other": {Role: config.RoleGeneric, PVEProfile: "production", Address: "other.example"},
	}}}
	name, host, err := findPVEInventoryHost(cfg, "production", "node-a")
	if err != nil {
		t.Fatalf("find host: %v", err)
	}
	if name != "pve" || host.Address != "pve.example" {
		t.Fatalf("unexpected host: %q %+v", name, host)
	}

	cfg.Inventory.Hosts["pve2"] = config.InventoryHost{Role: config.RolePVE, PVENode: "node-a", PVEProfile: "production"}
	if _, _, err := findPVEInventoryHost(cfg, "production", "node-a"); err == nil {
		t.Fatal("ambiguous PVE hosts accepted")
	}
	if _, _, err := findPVEInventoryHost(cfg, "production", "node-b"); err == nil {
		t.Fatal("unmapped PVE node accepted")
	}
}

func TestInterpretContainerOSUpdate(t *testing.T) {
	result := &ansible.RunResult{
		Success: true,
		Hosts:   []ansible.HostResult{{Host: "pve", OK: 6}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {
				{Task: containerStatusTask, RC: intPtrCLI(0)},
				{Task: containerAPTTask, RC: intPtrCLI(0)},
				{Task: containerPackagesTask, RC: intPtrCLI(0), StdoutLines: []string{"Listing...", "openssl/stable 3.0 amd64 [upgradable from: 2.9]"}},
				{Task: containerSimTask, RC: intPtrCLI(0)},
				{Task: containerDpkgTask, RC: intPtrCLI(0)},
				{Task: containerRebootTask, RC: intPtrCLI(1)},
				{Task: containerRootTask, RC: intPtrCLI(0), StdoutLines: []string{"Filesystem 1024-blocks Used Available Capacity Mounted on", "/dev/root 100 91 9 91% /"}},
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

func TestInterpretContainerOSUpdateRejectsIncompleteCommandEvidence(t *testing.T) {
	zero := intPtrCLI(0)
	result := &ansible.RunResult{
		Success: true,
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {
				{Task: containerStatusTask, RC: zero},
				{Task: containerAPTTask, RC: zero},
				{Task: containerPackagesTask, RC: zero},
				{Task: containerSimTask, RC: zero},
				{Task: containerDpkgTask, RC: zero},
				{Task: containerRebootTask, RC: intPtrCLI(1)},
				{Task: containerRootTask, RC: zero},
			},
		},
	}
	state := interpretContainerOSUpdate(result, "pve")
	if state.EvidenceComplete {
		t.Fatal("empty root usage must not be complete evidence")
	}
	evidence := missingContainerUpdateEvidence(result, "pve")
	if len(evidence) != 1 || !strings.Contains(evidence[0], "missing root usage") {
		t.Fatalf("evidence = %v, want root usage diagnostic", evidence)
	}

	result.TaskOutcomes["pve"][0].RC = nil
	state = interpretContainerOSUpdate(result, "pve")
	if state.EvidenceComplete {
		t.Fatal("command result without return code must not be complete evidence")
	}
	evidence = missingContainerUpdateEvidence(result, "pve")
	if len(evidence) != 2 || !strings.Contains(evidence[0], "unusable") {
		t.Fatalf("evidence = %v, want unusable status diagnostic", evidence)
	}
}

func intPtrCLI(value int) *int { return &value }
