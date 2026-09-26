package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
)

// writeGateConfig writes an isolated config.yaml with the given current profile.
func writeGateConfig(t *testing.T, dir, current string, profiles map[string]config.Profile) {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	var b strings.Builder
	b.WriteString("version: 2\n")
	if current != "" {
		b.WriteString("current_profile: " + current + "\n")
	}
	b.WriteString("profiles:\n")
	for name, p := range profiles {
		b.WriteString("  " + name + ":\n")
		b.WriteString("    provider: " + p.Provider + "\n")
		b.WriteString("    endpoint: " + p.Endpoint + "\n")
		b.WriteString("    credential_ref: " + p.CredentialRef + "\n")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func gateTestProfiles() map[string]config.Profile {
	pveRef := "env:pbs-gate-pve"
	pbsRef := "env:pbs-gate-pbs"
	profiles := map[string]config.Profile{}
	profiles["pve"] = config.Profile{
		Provider:      config.ProviderProxmox,
		Endpoint:      "https://pve.invalid:8006",
		CredentialRef: pveRef,
	}
	profiles["pbs1"] = config.Profile{
		Provider:      config.ProviderPBS,
		Endpoint:      "https://pbs.invalid:8007",
		CredentialRef: pbsRef,
	}
	return profiles
}

// TestPBSGroupGatedOnPVEOnlyProfile verifies nit #35: every pbs subcommand on a
// PVE-only profile fails with one actionable message instead of a dead end.
func TestPBSGroupGatedOnPVEOnlyProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	cfgDir := filepath.Join(dir, "xdg", "nodex")
	writeGateConfig(t, cfgDir, "pve", gateTestProfiles())

	subcommands := [][]string{
		{"pbs", "status"},
		{"pbs", "version"},
		{"pbs", "subscription"},
		{"pbs", "certificates"},
		{"pbs", "datastore", "list"},
		{"pbs", "snapshot", "list"},
		{"pbs", "task", "list"},
		{"pbs", "verify", "list"},
		{"pbs", "prune", "list"},
		{"pbs", "sync", "list"},
		{"pbs", "garbage-collection"},
	}
	for _, args := range subcommands {
		var stdout, stderr bytes.Buffer
		err := Run(context.Background(), args, &stdout, &stderr)
		if err == nil {
			t.Errorf("Run(%v) succeeded, want unsupported-capability error", args)
			continue
		}
		var exitCoder *app.ExitCoder
		if !stderrors.As(err, &exitCoder) || exitCoder.ExitCode != app.ExitUnsupportedCap {
			t.Errorf("Run(%v) exit code = %v, want %v", args, err, app.ExitUnsupportedCap)
		}
		msg := err.Error()
		// The message must name the profile, the provider that lacks PBS support,
		// and the real remediation commands.
		if !strings.Contains(msg, `profile "pve"`) || !strings.Contains(msg, `provider "proxmox"`) {
			t.Errorf("Run(%v) error should name the profile and its provider, got: %v", args, msg)
		}
		if !strings.Contains(msg, "provider supports PBS") {
			t.Errorf("Run(%v) error should state the requirement, got: %v", args, msg)
		}
		if !strings.Contains(msg, "profile add") || !strings.Contains(msg, "--provider pbs") {
			t.Errorf("Run(%v) error should name the add remediation, got: %v", args, msg)
		}
		if !strings.Contains(msg, "profile use") {
			t.Errorf("Run(%v) error should name the select remediation, got: %v", args, msg)
		}
	}
}

// TestPBSGroupGateHonorsExplicitProfile verifies --profile overrides the current
// profile for the gate.
func TestPBSGroupGateHonorsExplicitProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	cfgDir := filepath.Join(dir, "xdg", "nodex")
	// Current profile is a valid PBS profile, but --profile selects the PVE one.
	writeGateConfig(t, cfgDir, "pbs1", gateTestProfiles())

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"pbs", "status", "--profile", "pve"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected gate to reject explicit PVE profile")
	}
	if !strings.Contains(err.Error(), `profile "pve"`) {
		t.Errorf("error should name the explicitly selected profile, got: %v", err)
	}
}

// TestPBSGroupGateAllowsPBSProfile verifies the gate does not fire for a genuine
// PBS profile; the command proceeds to credential/network handling instead.
func TestPBSGroupGateAllowsPBSProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	cfgDir := filepath.Join(dir, "xdg", "nodex")
	writeGateConfig(t, cfgDir, "pbs1", gateTestProfiles())

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"pbs", "status"}, &stdout, &stderr)
	if err == nil {
		t.Skip("PBS profile resolved far enough to succeed; nothing to assert")
	}
	if strings.Contains(err.Error(), "require a profile whose provider supports PBS") {
		t.Errorf("gate must not fire for a PBS profile, got: %v", err)
	}
}

// TestPBSGateUnknownProfileKeepsProfileError verifies a missing profile reports
// the profile problem rather than a misleading provider mismatch.
func TestPBSGateUnknownProfileKeepsProfileError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	cfgDir := filepath.Join(dir, "xdg", "nodex")
	writeGateConfig(t, cfgDir, "missing", gateTestProfiles())

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"pbs", "status"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected profile-not-found error")
	}
	if !strings.Contains(err.Error(), "profile") {
		t.Errorf("error should describe the profile problem, got: %v", err)
	}
	if strings.Contains(err.Error(), "require a profile whose provider supports PBS") {
		t.Errorf("provider gate should not mask a missing profile, got: %v", err)
	}
}

// TestPBSHelpUnaffectedByGate verifies help still lists subcommands regardless of
// the selected profile, so discovery still works.
func TestPBSHelpUnaffectedByGate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	cfgDir := filepath.Join(dir, "xdg", "nodex")
	writeGateConfig(t, cfgDir, "pve", gateTestProfiles())

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"help", "pbs"}, &stdout, &stderr); err != nil {
		t.Fatalf("help pbs failed: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"status", "datastore", "snapshot", "garbage-collection"} {
		if !strings.Contains(out, want) {
			t.Errorf("help pbs output missing %q", want)
		}
	}
}

// TestOnlyPBSGroupIsGated guards against the gate leaking onto other groups.
func TestOnlyPBSGroupIsGated(t *testing.T) {
	for name, cmd := range commands {
		if cmd.gate != nil && name != "pbs" {
			t.Errorf("command group %q has an unexpected gate", name)
		}
	}
	if commands["pbs"].gate == nil {
		t.Error("pbs group must be gated")
	}
}
