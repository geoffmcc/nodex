package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/maintenance"
)

// seedMaintenanceConfig seeds profiles, environments (mock providers), and
// an inventory with a mix of roles and criticalities.
func seedMaintenanceConfig(t *testing.T) {
	t.Helper()
	seedEnvironmentE2EConfig(t)
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	cfg, err := config.ReadFrom(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	cfg.Inventory = &config.Inventory{
		Hosts: map[string]config.InventoryHost{
			"web1": {
				Address: "web1.example.invalid", Role: "generic", Environment: "e2e-env",
				SSHUser: "automation", MaintenanceGroup: "guests",
			},
			"pve-primary": {
				Address: "pve.example.invalid", Role: "pve", Environment: "e2e-env",
				SSHUser: "automation", MaintenanceGroup: "hypervisors",
				Criticality: "critical", BackupRequired: true,
			},
			"standalone": {
				Address: "standalone.example.invalid", Role: "generic",
				SSHUser: "automation",
			},
		},
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// withCannedCheckUpdates replaces the Ansible seam for one test.
func withCannedCheckUpdates(t *testing.T, fn func(ctx context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error)) {
	t.Helper()
	prev := runCheckUpdates
	runCheckUpdates = fn
	t.Cleanup(func() { runCheckUpdates = prev })
}

func cannedHealthyResult(hosts []ansible.HostSpec) *ansible.RunResult {
	res := &ansible.RunResult{Operation: "check-updates", Success: true}
	res.TaskOutcomes = map[string][]ansible.TaskOutcome{}
	for _, h := range hosts {
		res.Hosts = append(res.Hosts, ansible.HostResult{Host: h.Name, OK: 7})
		res.TaskOutcomes[h.Name] = []ansible.TaskOutcome{
			{Task: "List upgradable packages", StdoutLines: []string{
				"Listing...",
				"openssl/stable-security 3.0.15-1 amd64 [upgradable from: 3.0.14-1]",
			}},
			{Task: "Simulate dist-upgrade", StdoutLines: []string{
				"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])",
			}},
			{Task: "Check reboot-required marker", StatExists: boolPtrCLI(false)},
			{Task: "List failed systemd units", StdoutLines: []string{}},
			{Task: "Report root filesystem usage", StdoutLines: []string{
				"Filesystem 1024-blocks Used Available Capacity Mounted on",
				"/dev/sda1 41152736 12345678 27000000 32% /",
			}},
		}
	}
	return res
}

func boolPtrCLI(b bool) *bool { return &b }

func TestMaintenanceInventory(t *testing.T) {
	seedMaintenanceConfig(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "inventory")
	if err != nil {
		t.Fatalf("maintenance inventory: %v", err)
	}
	for _, want := range []string{"web1", "pve-primary", "standalone"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("inventory missing %q:\n%s", want, stdout)
		}
	}
}

func TestMaintenanceInventoryFilters(t *testing.T) {
	seedMaintenanceConfig(t)
	tests := []struct {
		name    string
		args    []string
		want    []string
		exclude []string
	}{
		{"by environment", []string{"--environment", "e2e-env"}, []string{"web1", "pve-primary"}, []string{"standalone"}},
		{"by role", []string{"--role", "pve"}, []string{"pve-primary"}, []string{"web1", "standalone"}},
		{"by group", []string{"--group", "guests"}, []string{"web1"}, []string{"pve-primary"}},
		{"by host", []string{"--host", "standalone"}, []string{"standalone"}, []string{"web1", "pve-primary"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"--output", "json", "maintenance", "inventory"}, tt.args...)
			stdout, _, err := runPBSCommand(t, args...)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(stdout, w) {
					t.Errorf("missing %q:\n%s", w, stdout)
				}
			}
			for _, e := range tt.exclude {
				if strings.Contains(stdout, "\""+e+"\"") {
					t.Errorf("should not contain %q:\n%s", e, stdout)
				}
			}
		})
	}
}

func TestMaintenanceInventoryUnknownHost(t *testing.T) {
	seedMaintenanceConfig(t)
	_, _, err := runPBSCommand(t, "maintenance", "inventory", "--host", "ghost")
	if err == nil {
		t.Fatal("unknown host must be rejected")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitNotFound {
		t.Errorf("error = %v, want ExitNotFound", err)
	}
}

func TestMaintenanceStatusHappyPath(t *testing.T) {
	seedMaintenanceConfig(t)
	var gotHosts []string
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		for _, h := range hosts {
			gotHosts = append(gotHosts, h.Name)
		}
		return cannedHealthyResult(hosts), nil
	})

	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "status", "--host", "web1")
	if err != nil {
		t.Fatalf("maintenance status: %v", err)
	}
	if len(gotHosts) != 1 || gotHosts[0] != "web1" {
		t.Errorf("preflight ran against %v, want [web1]", gotHosts)
	}
	var res maintenanceStatusResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("parse: %v\n%s", err, stdout)
	}
	if len(res.Hosts) != 1 || res.Hosts[0].Host != "web1" {
		t.Fatalf("hosts = %+v", res.Hosts)
	}
	h := res.Hosts[0]
	if len(h.PendingUpdates) != 1 || h.PendingUpdates[0] != "openssl" {
		t.Errorf("pending updates = %v", h.PendingUpdates)
	}
	if len(h.SecurityUpdates) != 1 {
		t.Errorf("security updates = %v", h.SecurityUpdates)
	}
	if h.RootUsage != "32%" {
		t.Errorf("root usage = %q", h.RootUsage)
	}
}

func TestMaintenanceStatusPartialFailureExits11(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		res := cannedHealthyResult(hosts)
		res.Success = false
		res.PartialFailure = true
		res.Hosts[0].Unreachable = 1
		res.Hosts[0].Failed = true
		return res, nil
	})
	_, _, err := runPBSCommand(t, "maintenance", "status", "--environment", "e2e-env")
	if err == nil {
		t.Fatal("partial failure must exit non-zero")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Errorf("error = %v, want ExitPartialFailure", err)
	}
}

func TestMaintenanceStatusRequiresInventory(t *testing.T) {
	seedPBSE2EConfig(t) // no inventory section
	_, _, err := runPBSCommand(t, "maintenance", "status")
	if err == nil {
		t.Fatal("missing inventory must error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitConfig {
		t.Errorf("error = %v, want ExitConfig", err)
	}
}

func TestMaintenancePlanHappyPath(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})

	// standalone has no backup requirement and no environment: plan should
	// be blocker-free and exit 0.
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "plan",
		"--policy", "security-only", "--host", "standalone")
	if err != nil {
		t.Fatalf("maintenance plan: %v", err)
	}
	var plan maintenance.Plan
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("parse plan: %v\n%s", err, stdout)
	}
	if err := maintenance.Verify(plan, time.Now()); err != nil {
		t.Errorf("emitted plan fails verification: %v", err)
	}
	if plan.Policy != maintenance.PolicySecurityOnly || plan.RebootPolicy != maintenance.RebootPolicyNever {
		t.Errorf("plan fields wrong: %+v", plan)
	}
	if len(plan.Blockers) != 0 {
		t.Errorf("unexpected blockers: %v", plan.Blockers)
	}
	if len(plan.Hosts) != 1 || plan.Hosts[0].Name != "standalone" {
		t.Errorf("plan hosts = %+v", plan.Hosts)
	}

	// Tamper with the emitted plan: verification must fail.
	tampered := strings.Replace(stdout, "security-only", "approved-full-upgrade", 1)
	var tp maintenance.Plan
	if err := json.Unmarshal([]byte(tampered), &tp); err != nil {
		t.Fatalf("parse tampered: %v", err)
	}
	if err := maintenance.Verify(tp, time.Now()); err == nil {
		t.Error("tampered emitted plan passed verification")
	}
}

func TestMaintenancePlanBackupRequiredWithoutEnvironmentBlocks(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "plan",
		"--policy", "security-only", "--host", "pve-primary")
	if err == nil {
		t.Fatal("plan with unverifiable backup requirements must exit non-zero")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Fatalf("error = %v, want ExitPartialFailure", err)
	}
	var plan maintenance.Plan
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("parse plan: %v", err)
	}
	if plan.Backup.Satisfied {
		t.Error("backup must not be satisfied without environment linkage")
	}
	found := false
	for _, b := range plan.Blockers {
		if strings.Contains(b, "backup requirements cannot be verified") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected backup blocker, got %v", plan.Blockers)
	}
	if err := maintenance.Verify(plan, time.Now()); err != nil {
		t.Errorf("blocked plan must still verify structurally: %v", err)
	}
}

func TestMaintenancePlanEnvironmentBlockersPropagate(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})
	// The mock environment always has an active verificationjob and ct 200
	// without backups, so maintenance_safe is false.
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "plan",
		"--policy", "security-only", "--environment", "e2e-env")
	if err == nil {
		t.Fatal("plan against unsafe environment must exit non-zero")
	}
	var plan maintenance.Plan
	if jsonErr := json.Unmarshal([]byte(stdout), &plan); jsonErr != nil {
		t.Fatalf("parse plan: %v", jsonErr)
	}
	if plan.Infra.MaintenanceSafe {
		t.Error("infra snapshot must record unsafe environment")
	}
	if len(plan.Blockers) == 0 {
		t.Error("environment blockers must propagate into the plan")
	}
	if plan.Hosts[0].Name != "pve-primary" && plan.Hosts[1].Name != "pve-primary" {
		t.Errorf("environment filter should include pve-primary: %+v", plan.Hosts)
	}
}

func TestMaintenancePlanUnreachableHostBlocks(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		res := cannedHealthyResult(hosts)
		res.Hosts[0].Unreachable = 1
		res.Hosts[0].Failed = true
		res.Success = false
		return res, nil
	})
	_, _, err := runPBSCommand(t, "maintenance", "plan", "--policy", "security-only", "--host", "standalone")
	if err == nil {
		t.Fatal("unreachable host must produce a blocked plan")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Errorf("error = %v, want ExitPartialFailure", err)
	}
}

func TestMaintenancePlanUsageErrors(t *testing.T) {
	seedMaintenanceConfig(t)
	for _, args := range [][]string{
		{"maintenance", "plan"},                                                     // missing policy
		{"maintenance", "plan", "--policy", "yolo"},                                 // bad policy
		{"maintenance", "plan", "--policy", "security-only", "--expires-in", "5s"},  // too short
		{"maintenance", "plan", "--policy", "security-only", "--expires-in", "48h"}, // too long
		{"maintenance", "plan", "--policy", "security-only", "--batch-size", "0"},   // bad batch
		{"maintenance", "plan", "--policy", "security-only", "--batch-size", "11"},  // bad batch
		{"maintenance", "plan", "--policy", "security-only", "--bogus"},             // unknown flag
		{"maintenance", "status", "--bogus"},                                        // unknown flag
		{"maintenance", "inventory", "extra"},                                       // stray arg
	} {
		_, _, err := runPBSCommand(t, args...)
		if err == nil {
			t.Errorf("Run(%v) succeeded, want usage error", args)
			continue
		}
		var exitCode *app.ExitCoder
		if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
			t.Errorf("Run(%v) error = %v, want ExitUsage", args, err)
		}
	}
}

func TestMaintenanceStatusAnsibleMissing(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, _ []ansible.HostSpec) (*ansible.RunResult, error) {
		return nil, app.NewExitError(
			stderrors.New("maintenance preflight requires Ansible: ansible-playbook is not installed or not on PATH"),
			app.ExitIncompatibility,
		)
	})
	_, _, err := runPBSCommand(t, "maintenance", "status")
	if err == nil {
		t.Fatal("missing ansible must error")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitIncompatibility {
		t.Errorf("error = %v, want ExitIncompatibility", err)
	}
	if !strings.Contains(err.Error(), "requires Ansible") {
		t.Errorf("error should explain the Ansible dependency: %v", err)
	}
}

func TestMaintenancePlanTableOutput(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})
	stdout, _, err := runPBSCommand(t, "--output", "table", "maintenance", "plan",
		"--policy", "security-only", "--host", "standalone")
	if err != nil {
		t.Fatalf("plan table: %v", err)
	}
	for _, want := range []string{"Plan:", "mp-", "Digest:", "security-only", "never", "standalone"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("table output missing %q:\n%s", want, stdout)
		}
	}
}
