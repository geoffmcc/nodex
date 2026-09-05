package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
)

func TestParseSetupArgsRejectsSecrets(t *testing.T) {
	for _, args := range [][]string{{"--token", "secret-value"}, {"--token-secret=secret-value"}, {"--password", "secret-value"}} {
		_, err := parseSetupArgs(args)
		if err == nil || !strings.Contains(err.Error(), "does not accept secrets") {
			t.Errorf("parseSetupArgs(%v) error = %v", args, err)
		}
		if strings.Contains(err.Error(), "secret-value") {
			t.Errorf("secret leaked in error: %v", err)
		}
	}
}

func TestRunSetupNonInteractiveRequiresExplicitInputs(t *testing.T) {
	isolateConfigAndHome(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--non-interactive", "setup", "--endpoint", "https://example.test:8006"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected non-interactive setup to fail closed")
	}
	var exitCode *app.ExitCoder
	if !errors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Fatalf("error = %v, want usage error", err)
	}
	if _, statErr := config.ConfigPath(); statErr != nil {
		t.Fatal(statErr)
	}
	path, _ := config.ConfigPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("config was written on rejected setup: %v", statErr)
	}
}

func TestRunSetupWritesProfileWithoutSecrets(t *testing.T) {
	isolateConfigAndHome(t)
	var stdout, stderr bytes.Buffer
	args := []string{"--non-interactive", "setup", "--provider", "proxmox", "--profile", "lab", "--endpoint", "https://pve.example.test:8006", "--credential-ref", "keyring:lab"}
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("setup: %v", err)
	}
	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if cfg.CurrentProfile != "lab" || cfg.Profiles["lab"].CredentialRef != "keyring:lab" {
		t.Fatalf("unexpected setup config: %+v", cfg)
	}
	if strings.Contains(stdout.String()+stderr.String(), "secret") {
		t.Fatal("setup output exposed a secret")
	}
}

func TestRunSetupDoesNotReplaceProfileWithoutForce(t *testing.T) {
	isolateConfigAndHome(t)
	args := []string{"--non-interactive", "setup", "--provider", "proxmox", "--profile", "lab", "--endpoint", "https://pve.example.test:8006"}
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), args, &stdout, &stderr); err != nil {
		t.Fatalf("initial setup: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), args, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("replacement error = %v", err)
	}
}
