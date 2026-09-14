package cli

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"os"
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
				PVENode: "pve-primary",
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
	res := &ansible.RunResult{
		Operation:        "check-updates",
		EvidenceSchema:   ansible.EvidenceSchemaVersion,
		EvidenceContract: ansible.EvidenceContract,
		RequiredEvidence: []string{ansible.HostEvidenceDebian, ansible.HostEvidenceRefresh, ansible.HostEvidencePackages, ansible.HostEvidenceSimulation, ansible.HostEvidenceReboot, ansible.HostEvidenceFailedUnits, ansible.HostEvidenceRoot},
		EvidenceComplete: true,
		Success:          true,
		ExitCode:         0,
	}
	res.TaskOutcomes = map[string][]ansible.TaskOutcome{}
	for _, h := range hosts {
		res.Hosts = append(res.Hosts, ansible.HostResult{Host: h.Name, OK: 7})
		res.TaskOutcomes[h.Name] = []ansible.TaskOutcome{
			{EvidenceID: ansible.HostEvidenceDebian, Task: "renamed Debian task"},
			{EvidenceID: ansible.HostEvidenceRefresh, Task: "renamed refresh task"},
			{EvidenceID: ansible.HostEvidencePackages, Task: "renamed package task", StdoutLines: []string{
				"Listing...",
				"openssl/stable-security 3.0.15-1 amd64 [upgradable from: 3.0.14-1]",
			}},
			{EvidenceID: ansible.HostEvidenceSimulation, Task: "renamed simulation task", StdoutLines: []string{
				"Inst openssl [3.0.14-1] (3.0.15-1 Debian-Security:12/stable-security [amd64])",
			}},
			{EvidenceID: ansible.HostEvidenceReboot, Task: "renamed reboot task", StatExists: boolPtrCLI(false)},
			{EvidenceID: ansible.HostEvidenceFailedUnits, Task: "renamed failed-units task", StdoutLines: []string{}},
			{EvidenceID: ansible.HostEvidenceRoot, Task: "renamed root task", StdoutLines: []string{
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

func TestMaintenanceStatusUnsuccessfulRunExits11(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		res := cannedHealthyResult(hosts)
		res.Success = false
		return res, nil
	})
	_, _, err := runPBSCommand(t, "maintenance", "status", "--host", "web1")
	if err == nil {
		t.Fatal("unsuccessful run must exit non-zero")
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

func TestMaintenanceApplyRequiresExactConfirmation(t *testing.T) {
	seedMaintenanceConfig(t)
	withCannedCheckUpdates(t, func(_ context.Context, hosts []ansible.HostSpec) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "plan", "--policy", "security-only", "--host", "standalone")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	planPath := t.TempDir() + "/plan.json"
	if err := os.WriteFile(planPath, []byte(stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	withCannedMaintenanceOperation(t, func(_ context.Context, operation string, hosts []ansible.HostSpec, packages []string) (*ansible.RunResult, error) {
		return cannedHealthyResult(hosts), nil
	})
	_, _, err = runPBSCommand(t, "maintenance", "apply", "--plan", planPath)
	if err == nil || !strings.Contains(err.Error(), "confirmation refused") {
		t.Fatalf("without confirmation error = %v", err)
	}
}

func withCannedMaintenanceOperation(t *testing.T, fn func(context.Context, string, []ansible.HostSpec, []string) (*ansible.RunResult, error)) {
	t.Helper()
	prev := runMaintenanceOperation
	runMaintenanceOperation = fn
	t.Cleanup(func() { runMaintenanceOperation = prev })
}

func testMaintenancePlan(t *testing.T, hosts ...string) maintenance.Plan {
	t.Helper()
	now := time.Now()
	plan := maintenance.Plan{
		Schema:               maintenance.PlanSchemaVersion,
		PlanID:               "mp-test-receipt",
		CreatedAt:            now.Add(-time.Minute).Unix(),
		ExpiresAt:            now.Add(time.Hour).Unix(),
		Policy:               maintenance.PolicySecurityOnly,
		BatchSize:            1,
		RebootPolicy:         maintenance.RebootPolicyNever,
		SafetyClassification: "disruptive",
		Infra:                maintenance.InfraSnapshot{MaintenanceSafe: true, EvidenceComplete: true},
		Snapshot:             maintenance.SafetySnapshot{Version: 1, Hosts: map[string]maintenance.HostSnapshot{}, EvidenceComplete: true},
	}
	for _, name := range hosts {
		ph := maintenance.PlanHost{Name: name, Address: name + ".example.invalid", Role: "generic", Criticality: config.CriticalityStandard}
		if name == "web1" {
			ph.Environment, ph.Group = "e2e-env", "guests"
		}
		plan.Hosts = append(plan.Hosts, ph)
		plan.HostOrder = append(plan.HostOrder, name)
		plan.Snapshot.Hosts[name] = maintenance.HostSnapshot{Name: name, Address: ph.Address, Role: ph.Role, Criticality: ph.Criticality}
	}
	finalized, err := maintenance.Finalize(plan)
	if err != nil {
		t.Fatalf("finalize test plan: %v", err)
	}
	return finalized
}

func cannedMaintenanceResult(hosts []ansible.HostSpec, required []string) *ansible.RunResult {
	res := &ansible.RunResult{
		Operation:        "maintenance-test",
		EvidenceSchema:   ansible.EvidenceSchemaVersion,
		EvidenceContract: ansible.EvidenceContract,
		RequiredEvidence: append([]string(nil), required...),
		EvidenceComplete: true,
		Success:          true,
		ExitCode:         0,
		TaskOutcomes:     map[string][]ansible.TaskOutcome{},
	}
	for _, h := range hosts {
		res.Hosts = append(res.Hosts, ansible.HostResult{Host: h.Name, OK: len(required)})
		for _, evidenceID := range required {
			res.TaskOutcomes[h.Name] = append(res.TaskOutcomes[h.Name], ansible.TaskOutcome{EvidenceID: evidenceID})
		}
	}
	return res
}

func TestMaintenanceBatchDurablyCheckpointsEachHost(t *testing.T) {
	plan := testMaintenancePlan(t, "web1", "standalone")
	receipt := maintenance.NewReceipt(plan, time.Now())
	if err := receipt.Finalize(); err != nil {
		t.Fatalf("finalize receipt: %v", err)
	}
	path := t.TempDir() + "/receipt.json"
	if err := maintenance.SaveReceipt(path, receipt); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	selected := map[string]config.InventoryHost{
		"web1":       {Address: "web1.example.invalid", SSHUser: "automation"},
		"standalone": {Address: "standalone.example.invalid", SSHUser: "automation"},
	}
	withCannedMaintenanceOperation(t, func(_ context.Context, _ string, hosts []ansible.HostSpec, _ []string) (*ansible.RunResult, error) {
		if hosts[0].Name == "standalone" {
			return nil, stderrors.New("injected provider outage")
		}
		result := cannedMaintenanceResult(hosts, []string{
			ansible.HostEvidenceDebian,
			ansible.HostEvidenceRefresh,
			ansible.HostEvidenceUpdate,
			ansible.HostEvidenceUpdateReport,
		})
		result.Hosts[0].Changed = 1
		return result, nil
	})
	completed := map[string]bool{}
	err := executeMaintenanceBatch(context.Background(), path, &receipt, selected, plan, "apply-security-updates", []string{"web1", "standalone"}, completed)
	if err == nil {
		t.Fatal("batch with one provider error must fail")
	}
	persisted, err := maintenance.LoadReceipt(path)
	if err != nil {
		t.Fatalf("load checkpointed receipt: %v", err)
	}
	states := map[string]string{}
	for _, host := range persisted.Hosts {
		states[host.Host] = host.State
	}
	if states["standalone"] != "unknown" || states["web1"] != "succeeded" {
		t.Fatalf("checkpointed host states = %v", states)
	}
	if len(persisted.Events) < 4 {
		t.Fatalf("events = %d, want initial and per-host checkpoints", len(persisted.Events))
	}
	for _, host := range persisted.Hosts {
		if host.Host == "web1" && host.Changed != 1 {
			t.Fatalf("web1 host counters = %+v, want changed=1", host)
		}
	}
}

func TestMaintenanceSecurityNoUpdatesAcceptsSkippedUpdateEvidence(t *testing.T) {
	result := cannedMaintenanceResult([]ansible.HostSpec{{Name: "web1"}}, []string{
		ansible.HostEvidenceDebian,
		ansible.HostEvidenceRefresh,
		ansible.HostEvidenceUpdate,
		ansible.HostEvidenceUpdateReport,
	})
	result.Operation = "apply-security-updates"
	for i := range result.TaskOutcomes["web1"] {
		if result.TaskOutcomes["web1"][i].EvidenceID == ansible.HostEvidenceUpdate {
			result.TaskOutcomes["web1"][i].Skipped = true
			result.TaskOutcomes["web1"][i].RC = nil
		}
	}

	state, success, detail := classifyMaintenanceResult(result, "web1")
	if !success || state != "succeeded" || len(detail) != 0 {
		t.Fatalf("security no-update result = state %q success %v detail %v", state, success, detail)
	}
}

func TestEnsureReceiptHostsForPlanCoversMissingHosts(t *testing.T) {
	plan := testMaintenancePlan(t, "web1", "standalone")
	receipt := maintenance.NewReceipt(plan, time.Now())
	receipt.Hosts = []maintenance.HostReceipt{{Host: "web1", Operation: "apply-security-updates", State: "unknown", Verification: "pending"}}
	if err := ensureReceiptHostsForPlan(&receipt, plan); err != nil {
		t.Fatalf("ensure receipt hosts: %v", err)
	}
	if len(receipt.Hosts) != len(plan.Hosts) {
		t.Fatalf("receipt hosts = %d, want %d", len(receipt.Hosts), len(plan.Hosts))
	}
	missing := receiptHost(&receipt, "standalone")
	if missing.Operation != "apply-security-updates" || missing.State != "unknown" || missing.Verification != "unknown" {
		t.Fatalf("missing host was not made explicit: %+v", *missing)
	}
}

func TestMaintenanceReconcilePreservesUnknownMutationOutcome(t *testing.T) {
	seedMaintenanceConfig(t)
	plan := testMaintenancePlan(t, "web1")
	planPath := t.TempDir() + "/plan.json"
	planBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, planBytes, 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	receipt := maintenance.NewReceipt(plan, time.Now())
	receipt.Hosts = []maintenance.HostReceipt{{Host: "web1", Operation: "apply-security-updates", State: "unknown", Verification: "pending"}}
	if err := receipt.Finalize(); err != nil {
		t.Fatalf("finalize receipt: %v", err)
	}
	receiptPath := t.TempDir() + "/receipt.json"
	if err := maintenance.SaveReceipt(receiptPath, receipt); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	withCannedMaintenanceOperation(t, func(_ context.Context, operation string, hosts []ansible.HostSpec, _ []string) (*ansible.RunResult, error) {
		if operation != "verify-maintenance" {
			t.Fatalf("operation = %q, want verify-maintenance", operation)
		}
		return cannedMaintenanceResult(hosts, []string{
			ansible.HostEvidenceConnectivity,
			ansible.HostEvidenceReboot,
			ansible.HostEvidenceFailedUnits,
			ansible.HostEvidenceRoot,
		}), nil
	})
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "reconcile", "--plan", planPath, "--receipt", receiptPath)
	if err == nil {
		t.Fatal("reconcile must not claim an interrupted mutation succeeded")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Fatalf("error = %v, want ExitPartialFailure", err)
	}
	var report maintenanceReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("parse reconcile report: %v\n%s", err, stdout)
	}
	if report.State != "unknown" || !report.Verified || !strings.Contains(report.Error, "outcome remains unknown") {
		t.Fatalf("reconcile report = %+v", report)
	}
	persisted, err := maintenance.LoadReceipt(receiptPath)
	if err != nil {
		t.Fatalf("load reconciled receipt: %v", err)
	}
	if persisted.State != "unknown" || persisted.Hosts[0].Verification != "succeeded" {
		t.Fatalf("reconciled receipt = %+v", persisted)
	}
}

func TestMaintenanceReconcileIncompleteEvidenceStaysUnknown(t *testing.T) {
	seedMaintenanceConfig(t)
	plan := testMaintenancePlan(t, "web1")
	planPath := t.TempDir() + "/plan.json"
	planBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if err := os.WriteFile(planPath, planBytes, 0o600); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	receipt := maintenance.NewReceipt(plan, time.Now())
	receipt.Hosts = []maintenance.HostReceipt{{Host: "web1", Operation: "apply-security-updates", State: "unknown", Verification: "pending"}}
	if err := receipt.Finalize(); err != nil {
		t.Fatalf("finalize receipt: %v", err)
	}
	receiptPath := t.TempDir() + "/receipt.json"
	if err := maintenance.SaveReceipt(receiptPath, receipt); err != nil {
		t.Fatalf("save receipt: %v", err)
	}
	withCannedMaintenanceOperation(t, func(_ context.Context, _ string, hosts []ansible.HostSpec, _ []string) (*ansible.RunResult, error) {
		result := cannedMaintenanceResult(hosts, []string{ansible.HostEvidenceConnectivity})
		result.Success = false
		result.EvidenceComplete = false
		result.ParseError = "required ansible evidence is missing"
		return result, nil
	})
	stdout, _, err := runPBSCommand(t, "--output", "json", "maintenance", "reconcile", "--plan", planPath, "--receipt", receiptPath)
	if err == nil {
		t.Fatal("incomplete evidence must remain non-successful")
	}
	var report maintenanceReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("parse reconcile report: %v\n%s", err, stdout)
	}
	if report.State != "unknown" || report.Verified || !strings.Contains(report.Error, "unknown") {
		t.Fatalf("incomplete-evidence report = %+v", report)
	}
}
