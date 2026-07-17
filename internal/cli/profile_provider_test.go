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

func TestParseProfileAddArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantName     string
		wantProvider string
		wantErr      bool
	}{
		{"name only defaults to proxmox", []string{"home"}, "home", "proxmox", false},
		{"provider flag", []string{"backup", "--provider", "pbs"}, "backup", "pbs", false},
		{"provider equals form", []string{"backup", "--provider=pbs"}, "backup", "pbs", false},
		{"provider before name", []string{"--provider", "pbs", "backup"}, "backup", "pbs", false},
		{"missing name", []string{"--provider", "pbs"}, "", "", true},
		{"missing provider value", []string{"backup", "--provider"}, "", "", true},
		{"empty provider equals", []string{"backup", "--provider="}, "", "", true},
		{"flag as provider value", []string{"backup", "--provider", "--quiet"}, "", "", true},
		{"unknown flag", []string{"backup", "--bogus"}, "", "", true},
		{"two names", []string{"one", "two"}, "", "", true},
		{"no args", nil, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, provider, err := parseProfileAddArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseProfileAddArgs(%v) = %q, %q, nil; want error", tt.args, name, provider)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseProfileAddArgs(%v): %v", tt.args, err)
			}
			if name != tt.wantName || provider != tt.wantProvider {
				t.Errorf("parseProfileAddArgs(%v) = %q, %q; want %q, %q",
					tt.args, name, provider, tt.wantName, tt.wantProvider)
			}
		})
	}
}

func TestRun_ProfileAddPBSProvider(t *testing.T) {
	isolateConfigAndHome(t)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--non-interactive", "init"}, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if err := Run(context.Background(), []string{"profile", "add", "backup", "--provider", "pbs"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile add --provider pbs: %v", err)
	}

	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	p, ok := cfg.Profiles["backup"]
	if !ok {
		t.Fatal("profile backup not created")
	}
	if p.Provider != config.ProviderPBS {
		t.Errorf("provider = %q, want %q", p.Provider, config.ProviderPBS)
	}
}

func TestRun_ProfileAddUnknownProviderRejected(t *testing.T) {
	isolateConfigAndHome(t)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--non-interactive", "init"}, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v", err)
	}

	err := Run(context.Background(), []string{"profile", "add", "cloud", "--provider", "openstack"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected unknown provider to be rejected")
	}
	var exitCode *app.ExitCoder
	if !stderrors.As(err, &exitCode) || exitCode.ExitCode != app.ExitUsage {
		t.Errorf("error = %v, want ExitUsage", err)
	}
	if !strings.Contains(err.Error(), "proxmox") || !strings.Contains(err.Error(), "pbs") {
		t.Errorf("error should list known providers, got: %v", err)
	}

	cfg, cfgErr := config.Read()
	if cfgErr != nil {
		t.Fatalf("read config: %v", cfgErr)
	}
	if _, exists := cfg.Profiles["cloud"]; exists {
		t.Error("rejected profile must not be written to config")
	}
}

func TestRun_ProfileAddProviderCaseNormalized(t *testing.T) {
	isolateConfigAndHome(t)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--non-interactive", "init"}, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := Run(context.Background(), []string{"profile", "add", "backup", "--provider", "PBS"}, &stdout, &stderr); err != nil {
		t.Fatalf("profile add --provider PBS: %v", err)
	}
	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := cfg.Profiles["backup"].Provider; got != config.ProviderPBS {
		t.Errorf("provider = %q, want normalized %q", got, config.ProviderPBS)
	}
}
