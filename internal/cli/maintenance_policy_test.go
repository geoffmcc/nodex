package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/maintenance"
)

func writePolicyInventory(t *testing.T, cfgDir string) {
	t.Helper()
	body := `version: 2
inventory:
  hosts:
    guest1:
      address: 192.0.2.41
      role: generic
      ssh_user: automation
      unattended_security_updates: true
    dns1:
      address: 192.0.2.53
      role: dns
      ssh_user: automation
`
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func policyInspectionResult(host string, state policyInspection) *ansible.RunResult {
	message, _ := json.Marshal(state)
	return &ansible.RunResult{
		Operation: "inspect-security-policy", Success: true, EvidenceComplete: true,
		Hosts: []ansible.HostResult{{Host: host, OK: 1}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			host: {{EvidenceID: ansible.SecurityPolicyEvidenceInspect, Message: string(message)}},
		},
	}
}

func policyMutationResult(operation, host string, evidenceID string) *ansible.RunResult {
	return &ansible.RunResult{
		Operation: operation, Success: true, EvidenceComplete: true,
		Hosts: []ansible.HostResult{{Host: host, OK: 1, Changed: 1}},
		TaskOutcomes: map[string][]ansible.TaskOutcome{
			host: {{EvidenceID: evidenceID, Message: "completed"}},
		},
	}
}

func baselinePolicyInspection() policyInspection {
	return policyInspection{
		Distribution: "Debian", DistributionRelease: "bookworm",
		AllowedOrigins:         []string{`Unattended-Upgrade::Allowed-Origins:: "o=Debian,a=bookworm-security,l=Debian-Security";`},
		NonSecurityOriginCount: 1, APTConfigRC: 0,
		PackageInstalled: false, TimerEnabled: "not-found", TimerEnabledRC: 1,
		TimerActive: "not-found", TimerActiveRC: 1,
	}
}

func finalizedSecurityPolicyPlan(t *testing.T, host config.InventoryHost) maintenance.SecurityPolicyPlan {
	t.Helper()
	policyHost := securityPolicyHostFromInventory("guest1", host)
	inspection := baselinePolicyInspection()
	populateSecurityPolicyHost(&policyHost, inspection)
	if len(policyHost.Blockers) != 0 {
		t.Fatalf("policy host blockers: %v", policyHost.Blockers)
	}
	now := time.Now()
	plan := maintenance.SecurityPolicyPlan{
		Schema: maintenance.SecurityPolicyPlanSchema, PlanID: "msp-test-plan",
		CreatedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
		Operation: "configure", RebootPolicy: maintenance.RebootPolicyNever,
		SafetyClassification: "disruptive", Hosts: []maintenance.SecurityPolicyHost{policyHost},
	}
	if err := plan.Finalize(); err != nil {
		t.Fatalf("finalize security policy plan: %v", err)
	}
	return plan
}

func TestMaintenancePolicyPlanIsExplicitOptInAndShowsSafeConfig(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writePolicyInventory(t, cfgDir)
	previous := runSecurityPolicyAnsible
	t.Cleanup(func() { runSecurityPolicyAnsible = previous })
	called := false
	runSecurityPolicyAnsible = func(_ context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
		called = true
		if request.Operation != "inspect-security-policy" || len(request.Hosts) != 1 || request.Hosts[0].Name != "guest1" {
			t.Fatalf("policy inspection scope = %+v", request)
		}
		return policyInspectionResult("guest1", baselinePolicyInspection()), nil
	}
	var stdout, stderr strings.Builder
	err := Run(context.Background(), []string{"--output", "json", "maintenance", "policy", "plan"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("maintenance policy plan: %v; stderr=%s", err, stderr.String())
	}
	if !called {
		t.Fatal("policy planner did not inspect the opted-in host")
	}
	var plan maintenance.SecurityPolicyPlan
	if err := json.Unmarshal([]byte(stdout.String()), &plan); err != nil {
		t.Fatalf("decode policy plan: %v", err)
	}
	if err := plan.Verify(time.Now()); err != nil {
		t.Fatalf("verify policy plan: %v", err)
	}
	if len(plan.Hosts) != 1 || plan.Hosts[0].Name != "guest1" || len(plan.ExcludedHosts) != 1 || plan.ExcludedHosts[0].Name != "dns1" {
		t.Fatalf("policy scope = hosts:%+v excluded:%+v", plan.Hosts, plan.ExcludedHosts)
	}
	if !strings.Contains(plan.Hosts[0].AfterConfig, "bookworm-security") || strings.Contains(plan.Hosts[0].AfterConfig, "bookworm-updates") {
		t.Fatalf("generated config is not security-only:\n%s", plan.Hosts[0].AfterConfig)
	}
	if !strings.Contains(plan.Hosts[0].AfterConfig, `Automatic-Reboot "false"`) {
		t.Fatal("generated unattended policy did not disable automatic reboots")
	}
	var textOut, textErr strings.Builder
	if err := Run(context.Background(), []string{"--output", "table", "maintenance", "policy", "plan", "--host", "guest1"}, &textOut, &textErr); err != nil {
		t.Fatalf("text policy plan: %v; stderr=%s", err, textErr.String())
	}
	if !strings.Contains(textOut.String(), "+++ guest1: "+ansible.SecurityPolicyConfigPath) || !strings.Contains(textOut.String(), `+Unattended-Upgrade::Automatic-Reboot "false";`) {
		t.Fatalf("text policy plan omitted the exact config diff:\n%s", textOut.String())
	}
}

func TestMaintenancePolicyPlanDerivesSafeOriginsWhenPackageIsNotInstalled(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writePolicyInventory(t, cfgDir)
	previous := runSecurityPolicyAnsible
	t.Cleanup(func() { runSecurityPolicyAnsible = previous })
	inspection := baselinePolicyInspection()
	inspection.AllowedOrigins = nil
	inspection.OriginsPatterns = nil
	inspection.NonSecurityOriginCount = 0
	runSecurityPolicyAnsible = func(_ context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
		return policyInspectionResult(request.Hosts[0].Name, inspection), nil
	}
	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"--output", "json", "maintenance", "policy", "plan", "--host", "guest1"}, &stdout, &stderr); err != nil {
		t.Fatalf("policy plan without unattended-upgrades package: %v; stderr=%s", err, stderr.String())
	}
	var plan maintenance.SecurityPolicyPlan
	if err := json.Unmarshal([]byte(stdout.String()), &plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	host := plan.Hosts[0]
	if len(host.BeforeSecurityAllowedOrigins)+len(host.BeforeOriginsPatterns) != 0 {
		t.Fatalf("plan claims prior unattended origins existed: %+v", host)
	}
	if len(host.SecurityOriginsPatterns) == 0 || !strings.Contains(host.SecurityOriginsPatterns[0], "${distro_codename}-security") {
		t.Fatalf("security-only Debian fallback missing: %+v", host.SecurityOriginsPatterns)
	}
}

func TestMaintenancePolicyApplyAndRestoreRequireExplicitConfirmation(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writePolicyInventory(t, cfgDir)
	cfg, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}
	host := cfg.Inventory.Hosts["guest1"]
	plan := finalizedSecurityPolicyPlan(t, host)
	planPath := filepath.Join(t.TempDir(), "security-policy.json")
	planBytes, _ := json.MarshalIndent(plan, "", "  ")
	if err := os.WriteFile(planPath, append(planBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	receipts := filepath.Join(t.TempDir(), "receipts")
	current := baselinePolicyInspection()
	previous := runSecurityPolicyAnsible
	t.Cleanup(func() { runSecurityPolicyAnsible = previous })
	runSecurityPolicyAnsible = func(_ context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
		hostName := request.Hosts[0].Name
		switch request.Operation {
		case "inspect-security-policy":
			return policyInspectionResult(hostName, current), nil
		case "apply-security-policy":
			if request.PolicyPlanID != plan.PlanID || request.PolicyConfig != plan.Hosts[0].AfterConfig || request.PolicyBeforeExists {
				t.Fatalf("apply policy vars = %+v", request)
			}
			current = policyInspection{
				Distribution: "Debian", DistributionRelease: "bookworm",
				AllowedOrigins:             []string{`Unattended-Upgrade::Allowed-Origins:: "o=Debian,a=bookworm-security,l=Debian-Security";`},
				PeriodicUpdatePackageLists: `APT::Periodic::Update-Package-Lists "1";`,
				PeriodicUnattendedUpgrade:  `APT::Periodic::Unattended-Upgrade "1";`,
				AutomaticReboot:            `Unattended-Upgrade::Automatic-Reboot "false";`,
				APTConfigRC:                0, ManagedFileExists: true, ManagedFileOwned: true,
				ManagedFileChecksum:   plan.Hosts[0].AfterConfigChecksum,
				ManagedFileContentB64: base64.StdEncoding.EncodeToString([]byte(plan.Hosts[0].AfterConfig)),
				PackageInstalled:      true, TimerEnabled: "enabled", TimerEnabledRC: 0,
				TimerActive: "active", TimerActiveRC: 0,
			}
			return policyMutationResult(request.Operation, hostName, ansible.SecurityPolicyEvidenceApply), nil
		case "restore-security-policy":
			if request.PolicyPlanID != plan.PlanID || !request.PolicyCurrentExists || request.PolicyCurrentChecksum != plan.Hosts[0].AfterConfigChecksum {
				t.Fatalf("restore policy vars = %+v", request)
			}
			current = policyInspection{
				Distribution: "Debian", DistributionRelease: "bookworm",
				AllowedOrigins:         []string{`Unattended-Upgrade::Allowed-Origins:: "o=Debian,a=bookworm-security,l=Debian-Security";`},
				NonSecurityOriginCount: 1, APTConfigRC: 0, PackageInstalled: true,
				TimerEnabled: "disabled", TimerEnabledRC: 1,
				TimerActive: "inactive", TimerActiveRC: 3,
			}
			return policyMutationResult(request.Operation, hostName, ansible.SecurityPolicyEvidenceRestore), nil
		default:
			t.Fatalf("unexpected Ansible operation %q", request.Operation)
			return nil, nil
		}
	}

	var noOut, noErr strings.Builder
	if err := Run(context.Background(), []string{"maintenance", "policy", "apply", "--plan", planPath}, &noOut, &noErr); err == nil || !strings.Contains(err.Error(), "requires --yes --force") {
		t.Fatalf("apply without confirmation err=%v", err)
	}
	var applyOut, applyErr strings.Builder
	err = Run(context.Background(), []string{"--output", "json", "--yes", "--force", "--confirm-target", plan.PlanID, "maintenance", "policy", "apply", "--plan", planPath, "--receipt-dir", receipts}, &applyOut, &applyErr)
	if err != nil {
		t.Fatalf("apply policy: %v; stderr=%s", err, applyErr.String())
	}
	var applied maintenance.Receipt
	if err := json.Unmarshal([]byte(applyOut.String()), &applied); err != nil {
		t.Fatalf("decode apply receipt: %v", err)
	}
	if applied.State != "succeeded" || len(applied.Hosts) != 1 || !applied.Hosts[0].Success || applied.Hosts[0].Verification != "succeeded" {
		t.Fatalf("apply receipt = %+v", applied)
	}

	var restoreOut, restoreErr strings.Builder
	err = Run(context.Background(), []string{"--output", "json", "--yes", "--force", "--confirm-target", plan.PlanID, "maintenance", "policy", "restore", "--plan", planPath, "--receipt-dir", receipts}, &restoreOut, &restoreErr)
	if err != nil {
		t.Fatalf("restore policy: %v; stderr=%s", err, restoreErr.String())
	}
	var restored maintenance.Receipt
	if err := json.Unmarshal([]byte(restoreOut.String()), &restored); err != nil {
		t.Fatalf("decode restore receipt: %v", err)
	}
	if restored.State != "succeeded" || len(restored.Hosts) != 1 || !restored.Hosts[0].Success || restored.Hosts[0].Verification != "succeeded" {
		t.Fatalf("restore receipt = %+v", restored)
	}
}

func TestMaintenancePolicyPlanDoesNotExposeOrReplaceNonCanonicalManagedFile(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writePolicyInventory(t, cfgDir)
	previous := runSecurityPolicyAnsible
	t.Cleanup(func() { runSecurityPolicyAnsible = previous })
	unsafeContent := maintenance.SecurityPolicyMarker + "\n# DO_NOT_LEAK\nAPT::Proxy \"secret-proxy\";\n"
	inspection := baselinePolicyInspection()
	inspection.ManagedFileExists = true
	inspection.ManagedFileOwned = true
	inspection.ManagedFileChecksum = maintenance.SecurityPolicyConfigChecksum(unsafeContent)
	inspection.ManagedFileContentB64 = base64.StdEncoding.EncodeToString([]byte(unsafeContent))
	runSecurityPolicyAnsible = func(_ context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
		return policyInspectionResult(request.Hosts[0].Name, inspection), nil
	}
	var stdout, stderr strings.Builder
	err := Run(context.Background(), []string{"--output", "json", "maintenance", "policy", "plan", "--host", "guest1"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("non-canonical managed configuration should block planning")
	}
	if strings.Contains(stdout.String(), "DO_NOT_LEAK") || strings.Contains(stdout.String(), "secret-proxy") {
		t.Fatalf("untrusted managed file content leaked into plan output: %s", stdout.String())
	}
	var plan maintenance.SecurityPolicyPlan
	if decodeErr := json.Unmarshal([]byte(stdout.String()), &plan); decodeErr != nil {
		t.Fatalf("decode blocked policy plan: %v; command error=%v; stderr=%s; stdout=%q", decodeErr, err, stderr.String(), stdout.String())
	}
	if len(plan.Blockers) == 0 || len(plan.Hosts) != 1 || plan.Hosts[0].BeforeConfig != "" {
		t.Fatalf("non-canonical policy file was not safely blocked: %+v", plan)
	}
}

func TestMaintenancePolicyApplyRefusesRemoteDriftBeforeMutation(t *testing.T) {
	_, cfgDir := isolatedConfigDir(t)
	writePolicyInventory(t, cfgDir)
	cfg, err := config.Read()
	if err != nil {
		t.Fatal(err)
	}
	plan := finalizedSecurityPolicyPlan(t, cfg.Inventory.Hosts["guest1"])
	planPath := filepath.Join(t.TempDir(), "security-policy.json")
	planBytes, _ := json.Marshal(plan)
	if err := os.WriteFile(planPath, planBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	current := baselinePolicyInspection()
	current.TimerActive = "active"
	current.TimerActiveRC = 0
	mutations := 0
	previous := runSecurityPolicyAnsible
	t.Cleanup(func() { runSecurityPolicyAnsible = previous })
	runSecurityPolicyAnsible = func(_ context.Context, request ansible.RunRequest) (*ansible.RunResult, error) {
		if request.Operation != "inspect-security-policy" {
			mutations++
		}
		return policyInspectionResult(request.Hosts[0].Name, current), nil
	}
	var stdout, stderr strings.Builder
	err = Run(context.Background(), []string{"--yes", "--force", "--confirm-target", plan.PlanID, "maintenance", "policy", "apply", "--plan", planPath}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "state changed") {
		t.Fatalf("apply after timer drift err=%v", err)
	}
	if mutations != 0 {
		t.Fatalf("policy mutation was submitted despite drift: %d", mutations)
	}
}

func TestDefaultSecurityAPTOriginsAreDistributionSpecific(t *testing.T) {
	debianPatterns, err := defaultSecurityAPTOrigins("Debian", "bookworm")
	if err != nil || len(debianPatterns) != 1 || !strings.Contains(debianPatterns[0], "${distro_codename}-security") {
		t.Fatalf("Debian security fallback patterns=%v err=%v", debianPatterns, err)
	}
	ubuntuPatterns, err := defaultSecurityAPTOrigins("Ubuntu", "jammy")
	if err != nil || len(ubuntuPatterns) != 3 || !strings.Contains(strings.Join(ubuntuPatterns, " "), "${distro_codename}-apps-security") {
		t.Fatalf("Ubuntu security fallback patterns=%v err=%v", ubuntuPatterns, err)
	}
	if _, err := defaultSecurityAPTOrigins("Fedora", "41"); err == nil {
		t.Fatal("unsupported distribution accepted for security origin fallback")
	}
}
