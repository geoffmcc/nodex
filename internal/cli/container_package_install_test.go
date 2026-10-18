package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/geoffmcc/nodex/internal/ansible"
	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestRunContainerPackageInstallUsesValidatedStructuredInputsAndReportsVerifiedSuccess(t *testing.T) {
	setupContainerPackageInstallFixture(t)
	original := runContainerPackageInstallAnsible
	t.Cleanup(func() { runContainerPackageInstallAnsible = original })

	var gotHost ansible.HostSpec
	var gotVMID int
	var gotPackages []string
	runContainerPackageInstallAnsible = func(_ context.Context, host ansible.HostSpec, vmid int, packages []string) (*ansible.RunResult, error) {
		gotHost, gotVMID, gotPackages = host, vmid, append([]string(nil), packages...)
		return &ansible.RunResult{
			Success: true, EvidenceComplete: true,
			Hosts: []ansible.HostResult{{Host: "pve-host", Changed: 1}},
		}, nil
	}

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--profile", "e2e", "--yes", "--force", "--output", "json",
		"container", "install", "e2e-node/200", "curl", "python3-venv",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("container install: %v (stderr %q)", err, stderr.String())
	}
	if gotVMID != 200 || !reflect.DeepEqual(gotPackages, []string{"curl", "python3-venv"}) {
		t.Fatalf("Ansible invocation vmid/packages = %d/%v", gotVMID, gotPackages)
	}
	if gotHost.Name != "pve-host" || gotHost.Address != "192.0.2.10" || gotHost.User != "root" || gotHost.Port != 22 || gotHost.KeyFile != "/fixture/pve.key" {
		t.Fatalf("Ansible host target = %+v", gotHost)
	}
	var result output.OperationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid structured result: %v; output=%q", err, stdout.String())
	}
	if !result.Success || result.Status != "OK" || result.Verification != "verified" || result.Target != "e2e-node/200" {
		t.Fatalf("operation result = %+v", result)
	}
}

func TestRunContainerPackageInstallFailsClosedBeforeAnsible(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		status     string
		wantCode   int
		wantRunner bool
	}{
		{name: "confirmation required", args: []string{"--profile", "e2e", "--non-interactive", "container", "install", "e2e-node/200", "curl"}, wantCode: app.ExitUsage},
		{name: "stopped container", args: []string{"--profile", "e2e", "--yes", "--force", "container", "install", "e2e-node/200", "curl"}, status: "stopped", wantCode: app.ExitValidationError},
		{name: "shell metacharacter", args: []string{"--profile", "e2e", "--yes", "--force", "container", "install", "e2e-node/200", "curl;id"}, wantCode: app.ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupContainerPackageInstallFixture(t)
			previousStatus := e2eContainerListStatusOverride
			e2eContainerListStatusOverride = tt.status
			t.Cleanup(func() { e2eContainerListStatusOverride = previousStatus })
			original := runContainerPackageInstallAnsible
			t.Cleanup(func() { runContainerPackageInstallAnsible = original })
			calls := 0
			runContainerPackageInstallAnsible = func(context.Context, ansible.HostSpec, int, []string) (*ansible.RunResult, error) {
				calls++
				return &ansible.RunResult{Success: true, EvidenceComplete: true}, nil
			}

			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("unsafe/incomplete request unexpectedly succeeded")
			}
			var exitErr *app.ExitCoder
			if !errors.As(err, &exitErr) || exitErr.ExitCode != tt.wantCode {
				t.Fatalf("error = %v, want exit code %d", err, tt.wantCode)
			}
			if calls != 0 {
				t.Fatalf("Ansible runner called %d times before preflight passed", calls)
			}
		})
	}
}

func TestRunContainerPackageInstallClassifiesRemoteUncertaintyAndIncompleteEvidence(t *testing.T) {
	tests := []struct {
		name     string
		result   *ansible.RunResult
		runErr   error
		wantCode int
	}{
		{name: "remote outcome uncertain", runErr: errors.New("connection lost after command launch"), wantCode: app.ExitAmbiguousOutcome},
		{name: "missing evidence is partial", result: &ansible.RunResult{Success: true, EvidenceComplete: false}, wantCode: app.ExitPartialFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupContainerPackageInstallFixture(t)
			original := runContainerPackageInstallAnsible
			t.Cleanup(func() { runContainerPackageInstallAnsible = original })
			runContainerPackageInstallAnsible = func(context.Context, ansible.HostSpec, int, []string) (*ansible.RunResult, error) {
				return tt.result, tt.runErr
			}
			var stdout, stderr bytes.Buffer
			err := Run(context.Background(), []string{
				"--profile", "e2e", "--yes", "--force", "container", "install", "e2e-node/200", "curl",
			}, &stdout, &stderr)
			if err == nil {
				t.Fatal("uncertain or incomplete package installation unexpectedly succeeded")
			}
			var exitErr *app.ExitCoder
			if !errors.As(err, &exitErr) || exitErr.ExitCode != tt.wantCode {
				t.Fatalf("error = %v, want exit code %d", err, tt.wantCode)
			}
		})
	}
}

func setupContainerPackageInstallFixture(t *testing.T) {
	t.Helper()
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	cfg, err := config.Read()
	if err != nil {
		t.Fatalf("read fixture config: %v", err)
	}
	cfg.Inventory = &config.Inventory{Hosts: map[string]config.InventoryHost{
		"pve-host": {
			Address: "192.0.2.10", Role: config.RolePVE, PVENode: "e2e-node", PVEProfile: "e2e",
			SSHUser: "root", SSHPort: 22, SSHKeyFile: "/fixture/pve.key",
		},
	}}
	if err := config.Write(cfg); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
}
