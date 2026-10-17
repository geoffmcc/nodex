package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/config"
)

func TestContainerOSUpdateStatusPrefersAuthoritativeGuestStatus(t *testing.T) {
	e2eContainerListStatusOverride = "stopped"
	e2eContainerStatusOverride = "running"
	t.Cleanup(func() {
		e2eContainerListStatusOverride = ""
		e2eContainerStatusOverride = ""
	})
	status, err := containerStatusForOSUpdate(context.Background(), &e2eMockProvider{}, "e2e-node", 200)
	if err != nil {
		t.Fatalf("containerStatusForOSUpdate: %v", err)
	}
	if status != "running" {
		t.Fatalf("status = %q, want authoritative running status despite stale inventory", status)
	}
}

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

func TestValidateContainerPackageNames(t *testing.T) {
	for _, packages := range [][]string{{"curl"}, {"python3-venv", "libssl3:amd64"}} {
		if err := validateContainerPackageNames(packages); err != nil {
			t.Errorf("valid package names rejected %v: %v", packages, err)
		}
	}
	for _, packages := range [][]string{{}, {"curl", "curl"}, {"--option"}, {"curl;id"}, {"foo=1.2"}, {strings.Repeat("a", 129)}} {
		if err := validateContainerPackageNames(packages); err == nil {
			t.Errorf("invalid package names accepted: %v", packages)
		}
	}
}

func TestFindPVEInventoryHost(t *testing.T) {
	cfg := &config.Config{Inventory: &config.Inventory{Hosts: map[string]config.InventoryHost{
		"pve":   {Role: config.RolePVE, PVENode: "node-a", PVEProfile: "production", Address: "pve.example"},
		"other": {Role: config.RoleGeneric, PVEProfile: "production", Address: "other.example"},
		"mixed": {Role: config.RolePVE, PVENode: "node-b", PVEProfile: "production", PBSProfile: "other-profile", Address: "mixed.example"},
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
		t.Fatal("host with a conflicting PBS profile was accepted")
	}
	cfg.Environments = map[string]config.Environment{
		"mixed-env": {PVEProfile: "production", PBSProfile: "other-profile"},
	}
	cfg.Inventory.Hosts["mixed-env"] = config.InventoryHost{
		Role: config.RolePVE, PVENode: "node-c", PVEProfile: "production", Environment: "mixed-env", Address: "mixed-env.example",
	}
	if _, _, err := findPVEInventoryHost(cfg, "production", "node-c"); err == nil {
		t.Fatal("host with a conflicting environment profile was accepted")
	}
	if _, _, err := findPVEInventoryHost(cfg, "production", "node-d"); err == nil {
		t.Fatal("unmapped PVE node accepted")
	}
}

func TestInterpretContainerOSUpdate(t *testing.T) {
	result := &ansible.RunResult{
		Success:          true,
		EvidenceComplete: true,
		Hosts:            []ansible.HostResult{{Host: "pve", OK: 6}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {
				{EvidenceID: containerStatusTask, Task: "renamed status task", RC: intPtrCLI(0)},
				{EvidenceID: containerAPTTask, Task: "renamed apt task", RC: intPtrCLI(0)},
				{EvidenceID: containerRefreshTask, Task: "renamed refresh task", RC: intPtrCLI(0)},
				{EvidenceID: containerPackagesTask, Task: "renamed package task", RC: intPtrCLI(0), StdoutLines: []string{"Listing...", "openssl/stable 3.0 amd64 [upgradable from: 2.9]"}},
				{EvidenceID: containerSimTask, Task: "renamed simulation task", RC: intPtrCLI(0), StdoutLines: []string{"0 upgraded, 0 newly installed, 0 to remove and 1 not upgraded."}},
				{EvidenceID: containerHoldTask, Task: "renamed hold task", RC: intPtrCLI(0), StdoutLines: []string{"python3-distupgrade"}},
				{EvidenceID: containerDpkgTask, Task: "renamed dpkg task", RC: intPtrCLI(0)},
				{EvidenceID: containerRebootTask, Task: "renamed reboot task", RC: intPtrCLI(1)},
				{EvidenceID: containerRootTask, Task: "renamed root task", RC: intPtrCLI(0), StdoutLines: []string{"Filesystem 1024-blocks Used Available Capacity Mounted on", "/dev/root 100 91 9 91% /"}},
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
	if state.UpgradePlan != "0 upgraded, 0 newly installed, 0 to remove and 1 not upgraded." {
		t.Fatalf("upgrade plan = %q", state.UpgradePlan)
	}
	if len(state.HeldPackages) != 1 || state.HeldPackages[0] != "python3-distupgrade" {
		t.Fatalf("held packages = %v", state.HeldPackages)
	}
}

func TestParseContainerUpgradePlan(t *testing.T) {
	want := "2 upgraded, 1 newly installed, 0 to remove and 0 not upgraded."
	if got := parseContainerUpgradePlan([]string{"Reading package lists...", want}); got != want {
		t.Fatalf("parseContainerUpgradePlan() = %q, want %q", got, want)
	}
	if got := parseContainerUpgradePlan([]string{"Reading package lists..."}); got != "" {
		t.Fatalf("parseContainerUpgradePlan() = %q, want empty", got)
	}
}

func TestInterpretContainerOSUpdateRejectsIncompleteCommandEvidence(t *testing.T) {
	zero := intPtrCLI(0)
	result := &ansible.RunResult{
		Success:          true,
		EvidenceComplete: true,
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {
				{EvidenceID: containerStatusTask, RC: zero},
				{EvidenceID: containerAPTTask, RC: zero},
				{EvidenceID: containerRefreshTask, RC: zero},
				{EvidenceID: containerPackagesTask, RC: zero},
				{EvidenceID: containerSimTask, RC: zero},
				{EvidenceID: containerHoldTask, RC: zero},
				{EvidenceID: containerDpkgTask, RC: zero},
				{EvidenceID: containerRebootTask, RC: intPtrCLI(1)},
				{EvidenceID: containerRootTask, RC: zero},
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

func TestInterpretContainerOSUpdateRejectsIncompleteRun(t *testing.T) {
	result := &ansible.RunResult{
		Success:          true,
		EvidenceComplete: false,
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			"pve": {{EvidenceID: containerRootTask, RC: intPtrCLI(0), StdoutLines: []string{"/dev/root 100 1 99 1% /"}}},
		},
	}
	state := interpretContainerOSUpdate(result, "pve")
	if state.EvidenceComplete {
		t.Fatal("unsuccessful evidence collection must not be interpreted as complete")
	}
}

func intPtrCLI(value int) *int { return &value }
