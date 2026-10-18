package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

func TestRunVMPackageInstallUsesGuestAgentAndReportsVerifiedSuccess(t *testing.T) {
	isolateConfigAndHome(t)
	setupE2EConfig(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"--profile", "e2e", "--yes", "--force", "--output", "json", "vm", "install", "e2e-node/100", "curl", "python3-venv"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("vm install: %v (stderr %q)", err, stderr.String())
	}
	var result output.OperationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid result: %v; %q", err, stdout.String())
	}
	if !result.Success || result.Verification != "verified" || result.Target != "e2e-node/100" {
		t.Fatalf("result = %+v", result)
	}
}

type vmGuestExecCall struct {
	argv []string
	pid  int
}
type scriptedVMGuestExecutor struct {
	calls                []vmGuestExecCall
	statuses             map[int]*domain.VMGuestExecStatus
	submitErr, statusErr error
	statusReads          int
	removalPlan          bool
	noChangePlan         bool
}

func (e *scriptedVMGuestExecutor) VMGuestExec(_ context.Context, node string, vmid int, argv []string) (int, error) {
	if node != "node-a" || vmid != 321 {
		return 0, errors.New("unexpected target")
	}
	if e.submitErr != nil {
		return 0, e.submitErr
	}
	pid := len(e.calls) + 1
	e.calls = append(e.calls, vmGuestExecCall{argv: append([]string(nil), argv...), pid: pid})
	stdout := ""
	switch argv[0] {
	case "/usr/bin/apt-get":
		if containsArg(argv, "-s") {
			if e.removalPlan {
				stdout = "Remv core-package [1.0]\n"
			} else if !e.noChangePlan {
				stdout = "Inst curl [8.0]\n"
			}
		} else if containsArg(argv, "install") {
			if !e.noChangePlan {
				stdout = "1 newly installed, 0 upgraded, 0 removed\n"
			}
		}
	case "/usr/bin/dpkg-query":
		stdout = "install ok installed"
	}
	if e.statuses == nil {
		e.statuses = make(map[int]*domain.VMGuestExecStatus)
	}
	exit := 0
	e.statuses[pid] = &domain.VMGuestExecStatus{Exited: true, ExitCode: &exit, Stdout: stdout}
	return pid, nil
}

func (e *scriptedVMGuestExecutor) VMGuestExecStatus(_ context.Context, node string, vmid, pid int) (*domain.VMGuestExecStatus, error) {
	if node != "node-a" || vmid != 321 {
		return nil, errors.New("unexpected target")
	}
	e.statusReads++
	if e.statusErr != nil {
		return nil, e.statusErr
	}
	return e.statuses[pid], nil
}

func TestInstallVMPackagesUsesFixedArgvAndVerifiesRequestedPackages(t *testing.T) {
	exec := &scriptedVMGuestExecutor{}
	changed, err := installVMPackages(context.Background(), exec, "node-a", 321, []string{"curl", "python3-venv"})
	if err != nil {
		t.Fatalf("installVMPackages: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if len(exec.calls) != 6 {
		t.Fatalf("guest command count = %d, want preflight/update/simulate/install/two verifies", len(exec.calls))
	}
	wantSimulation := []string{"/usr/bin/apt-get", "-s", "--no-remove", "-o", "DPkg::Lock::Timeout=120", "install", "--", "curl", "python3-venv"}
	if !reflect.DeepEqual(exec.calls[2].argv, wantSimulation) {
		t.Fatalf("simulation argv = %#v", exec.calls[2].argv)
	}
	wantInstall := []string{"/usr/bin/env", "DEBIAN_FRONTEND=noninteractive", "/usr/bin/apt-get", "-qq", "-y", "--no-remove", "-o", "DPkg::Lock::Timeout=120", "install", "--", "curl", "python3-venv"}
	if !reflect.DeepEqual(exec.calls[3].argv, wantInstall) {
		t.Fatalf("install argv = %#v", exec.calls[3].argv)
	}
	if !reflect.DeepEqual(exec.calls[4].argv, []string{"/usr/bin/dpkg-query", "-W", "-f=${Status}\n", "--", "curl"}) || exec.calls[5].argv[len(exec.calls[5].argv)-1] != "python3-venv" {
		t.Fatalf("verification argv = %#v / %#v", exec.calls[4].argv, exec.calls[5].argv)
	}
}

func TestInstallVMPackagesRefusesAPTRemovalPlan(t *testing.T) {
	exec := &scriptedVMGuestExecutor{removalPlan: true}
	_, err := installVMPackages(context.Background(), exec, "node-a", 321, []string{"curl"})
	if err == nil || app.ExitCodeFromError(err) != app.ExitValidationError {
		t.Fatalf("error = %v, want removal refusal", err)
	}
	if len(exec.calls) != 3 {
		t.Fatalf("commands submitted = %d, want no install after simulation", len(exec.calls))
	}
}

func TestInstallVMPackagesReportsNoChangeWhenSimulationIsEmpty(t *testing.T) {
	exec := &scriptedVMGuestExecutor{noChangePlan: true}
	changed, err := installVMPackages(context.Background(), exec, "node-a", 321, []string{"curl"})
	if err != nil {
		t.Fatalf("installVMPackages: %v", err)
	}
	if changed {
		t.Fatal("changed=true for an empty APT simulation")
	}
}

func TestInstallVMPackagesPreservesUnknownRemoteOutcome(t *testing.T) {
	exec := &scriptedVMGuestExecutor{submitErr: errors.New("connection lost")}
	_, err := installVMPackages(context.Background(), exec, "node-a", 321, []string{"curl"})
	if err == nil || app.ExitCodeFromError(err) != app.ExitAmbiguousOutcome {
		t.Fatalf("error = %v, want ambiguous outcome", err)
	}
	if len(exec.calls) != 0 {
		t.Fatalf("commands unexpectedly recorded after submission error: %d", len(exec.calls))
	}
}

func TestInstallVMPackagesDoesNotResubmitWhenGuestStatusIsLost(t *testing.T) {
	exec := &scriptedVMGuestExecutor{statusErr: errors.New("connection lost while polling")}
	_, err := installVMPackages(context.Background(), exec, "node-a", 321, []string{"curl"})
	if err == nil || app.ExitCodeFromError(err) != app.ExitAmbiguousOutcome {
		t.Fatalf("error = %v, want ambiguous outcome", err)
	}
	if len(exec.calls) != 1 || exec.statusReads != 1 {
		t.Fatalf("submission/poll counts = %d/%d, want exactly one submission and one status read", len(exec.calls), exec.statusReads)
	}
}

func containsArg(argv []string, target string) bool {
	for _, arg := range argv {
		if arg == target {
			return true
		}
	}
	return false
}
