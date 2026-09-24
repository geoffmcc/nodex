package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
)

// runSeedCommand isolates the config home, writes the given config, and runs
// nodex against it.
func runSeedCommand(t *testing.T, cfg *config.Config, args ...string) (string, error) {
	t.Helper()
	isolateConfigAndHome(t)
	path, err := config.ConfigPath()
	if err != nil {
		return "", err
	}
	if err := config.WriteTo(cfg, path); err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	err = Run(context.Background(), args, &stdout, &stderr)
	return stdout.String(), err
}

// seedProfileOnlyConfig returns a config that has profiles but no explicit
// environments section, exercising the implicit-environment path.
func seedProfileOnlyConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.CurrentProfile = "prod"
	cfg.Profiles["prod"] = config.Profile{
		Provider:      config.ProviderProxmox,
		Endpoint:      "https://pve.example.invalid",
		CredentialRef: "env:prod",
	}
	return cfg
}

func TestEnvironment_ImplicitFromCurrentProfile(t *testing.T) {
	cfg := seedProfileOnlyConfig()
	stdout, err := runSeedCommand(t, cfg, "--output", "json", "environment", "list")
	if err != nil {
		t.Fatalf("environment list: %v", err)
	}
	for _, want := range []string{`"prod"`, `"pve_profile": "prod"`, `"source": "implicit"`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output missing %s:\n%s", want, stdout)
		}
	}
}

func TestEnvironment_ListImplicitTableNote(t *testing.T) {
	cfg := seedProfileOnlyConfig()
	stdout, err := runSeedCommand(t, cfg, "--output", "table", "environment", "list")
	if err != nil {
		t.Fatalf("environment list: %v", err)
	}
	for _, want := range []string{"Implicit environment", `"prod"`, "prod"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("table output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "No environments configured.") {
		t.Errorf("table output must not report a dead end when a profile is set:\n%s", stdout)
	}
}

func TestEnvironment_ListEmptyStaysEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	stdout, err := runSeedCommand(t, cfg, "--output", "json", "environment", "list")
	if err != nil {
		t.Fatalf("environment list: %v", err)
	}
	if strings.TrimSpace(stdout) != "[]" {
		t.Errorf("expected stable empty list [], got %q", stdout)
	}
}

func TestEnvironment_HealthResolvesImplicitEnv(t *testing.T) {
	cfg := seedProfileOnlyConfig()
	// A local refused connection makes evaluation fail fast and
	// deterministically without resolving an implicit environment into a
	// not-found dead end.
	p := cfg.Profiles["prod"]
	p.Endpoint = "https://127.0.0.1:1"
	cfg.Profiles["prod"] = p
	stdout, err := runSeedCommand(t, cfg, "--output", "json", "--timeout", "2s", "environment", "health", "prod")
	if err == nil {
		t.Fatalf("expected non-zero exit for unreachable profile, got nil\n%s", stdout)
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitPartialFailure {
		t.Errorf("error = %v, want ExitPartialFailure (implicit env resolved and evaluated)", err)
	}
}

func TestEnvironment_HealthUnknownNameHintsImplicit(t *testing.T) {
	cfg := seedProfileOnlyConfig()
	_, err := runSeedCommand(t, cfg, "environment", "health", "other")
	if err == nil {
		t.Fatal("expected config error for unknown environment")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitConfig {
		t.Errorf("error = %v, want ExitConfig", err)
	}
	if !strings.Contains(err.Error(), "derives") {
		t.Errorf("error should hint at the implicit environment, got: %v", err)
	}
}
