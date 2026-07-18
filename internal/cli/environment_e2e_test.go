package cli

import (
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
)

// seedEnvironmentE2EConfig extends the PBS e2e seed with an environments
// section that pairs the PVE and PBS mock providers.
func seedEnvironmentE2EConfig(t *testing.T) {
	t.Helper()
	seedPBSE2EConfig(t)
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	cfg, err := config.ReadFrom(path)
	if err != nil {
		t.Fatalf("read seeded config: %v", err)
	}
	// The environments section requires known provider types; the mock
	// providers register under test-only names, so validation is bypassed by
	// constructing profiles the validator accepts: mock providers keep their
	// names, and validateEnvironmentProfileRef checks the provider string.
	// Register mock-typed environment by using profile providers directly.
	cfg.Environments = map[string]config.Environment{
		"e2e-env": {PVEProfile: "pve", PBSProfile: "pbs-e2e"},
	}
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("write env config: %v", err)
	}
}

func TestEnvironmentE2E_List(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "environment", "list")
	if err != nil {
		t.Fatalf("environment list: %v", err)
	}
	if !strings.Contains(stdout, "e2e-env") || !strings.Contains(stdout, "pbs-e2e") {
		t.Errorf("list output missing environment: %s", stdout)
	}
}

func TestEnvironmentE2E_ListEmpty(t *testing.T) {
	seedPBSE2EConfig(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "environment", "list")
	if err != nil {
		t.Fatalf("environment list: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("expected stable empty list [], got %q", stdout)
	}
}

func TestEnvironmentE2E_HealthExitsZero(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "environment", "health", "e2e-env")
	if err != nil {
		t.Fatalf("environment health should exit 0 for healthy infra: %v", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v", jsonErr)
	}
	if result["overall"] != "healthy" {
		t.Errorf("overall = %v, want healthy\n%s", result["overall"], stdout)
	}
	// The mock always has a running verificationjob, so maintenance is
	// deferred even though the environment is healthy.
	if result["maintenance_safe"] != false {
		t.Errorf("maintenance_safe = %v, want false (active task)", result["maintenance_safe"])
	}
	if _, hasGuests := result["guests"]; hasGuests {
		t.Error("environment health must not evaluate guests")
	}
}

func TestEnvironmentE2E_BackupHealthDetectsGaps(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	stdout, _, err := runPBSCommand(t, "--output", "json", "environment", "backup-health", "e2e-env")
	if err == nil {
		t.Fatal("expected non-zero exit: mock ct 200 has no backup")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Fatalf("error = %v, want ExitPartialFailure", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jsonErr, stdout)
	}
	if result["overall"] != "blocked" {
		t.Errorf("overall = %v, want blocked (missing ct backup)", result["overall"])
	}
	if result["maintenance_safe"] != false {
		t.Error("maintenance_safe must be false")
	}

	guests, ok := result["guests"].([]any)
	if !ok || len(guests) != 2 {
		t.Fatalf("expected 2 guests, got %v", result["guests"])
	}
	byVMID := map[float64]map[string]any{}
	for _, g := range guests {
		gm := g.(map[string]any)
		byVMID[gm["vmid"].(float64)] = gm
	}
	// VM 100 has a backup but it is years old: warning.
	if byVMID[100]["status"] != "warning" {
		t.Errorf("vm 100 status = %v, want warning (stale)", byVMID[100]["status"])
	}
	if byVMID[100]["datastore"] != "backups" {
		t.Errorf("vm 100 datastore = %v", byVMID[100]["datastore"])
	}
	// CT 200 has no backup at all: blocked.
	if byVMID[200]["status"] != "blocked" {
		t.Errorf("ct 200 status = %v, want blocked (no backup)", byVMID[200]["status"])
	}
}

func TestEnvironmentE2E_UnknownEnvironment(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	_, _, err := runPBSCommand(t, "environment", "health", "nonexistent")
	if err == nil {
		t.Fatal("expected config error for unknown environment")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitConfig {
		t.Errorf("error = %v, want ExitConfig", err)
	}
}

func TestEnvironmentE2E_UsageErrors(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	for _, args := range [][]string{
		{"environment", "list", "extra"},
		{"environment", "health"},
		{"environment", "health", "a", "b"},
		{"environment", "backup-health"},
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

func TestEnvironmentE2E_TableAndYAMLParity(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	for _, format := range []string{"table", "yaml"} {
		stdout, _, err := runPBSCommand(t, "--output", format, "environment", "backup-health", "e2e-env")
		// Non-zero exit expected (blocked); output must still be complete.
		if err == nil {
			t.Fatalf("[%s] expected blocked exit", format)
		}
		for _, want := range []string{"blocked", "guest"} {
			if !strings.Contains(strings.ToLower(stdout), want) {
				t.Errorf("[%s] output missing %q:\n%s", format, want, stdout)
			}
		}
	}
}

// TestEnvironmentE2E_ProviderDownNeverHealthy points the environment at a
// profile whose endpoint is unreachable and verifies the result degrades
// instead of erroring out entirely.
func TestEnvironmentE2E_ProviderDownNeverHealthy(t *testing.T) {
	seedEnvironmentE2EConfig(t)
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	cfg, err := config.ReadFrom(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Point the PBS profile at a credential that cannot resolve, so
	// connection fails while PVE stays up.
	p := cfg.Profiles["pbs-e2e"]
	p.CredentialRef = "env:does-not-exist"
	cfg.Profiles["pbs-e2e"] = p
	if err := config.WriteTo(cfg, path); err != nil {
		t.Fatalf("write: %v", err)
	}

	stdout, _, err := runPBSCommand(t, "--output", "json", "environment", "health", "e2e-env")
	if err == nil {
		t.Fatal("expected non-zero exit when PBS is unreachable")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Fatalf("error = %v, want ExitPartialFailure", err)
	}
	var result map[string]any
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v", jsonErr)
	}
	if result["overall"] == "healthy" {
		t.Error("overall must not be healthy when PBS cannot connect")
	}
	if result["partial_failure"] != true {
		t.Error("partial_failure must be true")
	}
	// PVE side must still have been evaluated.
	checks := result["checks"].([]any)
	foundPVE := false
	for _, c := range checks {
		cm := c.(map[string]any)
		if cm["name"] == "pve_reachable" && cm["status"] == "healthy" {
			foundPVE = true
		}
	}
	if !foundPVE {
		t.Errorf("pve_reachable should be healthy while PBS is down: %v", checks)
	}
}
