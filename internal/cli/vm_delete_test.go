package cli

import (
	"bytes"
	"context"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
)

func TestValidateVMDeleteState(t *testing.T) {
	tests := []struct {
		name string
		vms  []domain.VM
		exit int
		want string
	}{
		{
			name: "running VM gives stop remedy",
			vms:  []domain.VM{{ID: "pve1/100", Status: "running"}},
			exit: app.ExitConflict,
			want: "nodex vm stop pve1/100",
		},
		{
			name: "stopped VM can proceed",
			vms:  []domain.VM{{ID: "pve1/100", Status: "stopped"}},
		},
		{
			name: "absent VM is not found",
			exit: app.ExitNotFound,
			want: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVMDeleteState(tt.vms, "pve1/100")
			if tt.exit == 0 {
				if err != nil {
					t.Fatalf("validateVMDeleteState: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
			var exitErr *app.ExitCoder
			if !stderrors.As(err, &exitErr) || exitErr.ExitCode != tt.exit {
				t.Fatalf("error = %v, want exit %d", err, tt.exit)
			}
		})
	}
}

func TestRunVMDeleteHelpDocumentsSafetyAndPrecondition(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"vm", "delete", "--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("vm delete --help: %v (stderr %q)", err, stderr.String())
	}
	for _, text := range []string{"stopped VM", "--yes", "--force", "--confirm-target", "must be stopped first"} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("help missing %q:\n%s", text, stdout.String())
		}
	}
}

func TestVMStartAndDeleteConfirmationUseSameResourceCase(t *testing.T) {
	seedLogE2E(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{"vm", "start", "e2e-node/100"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "Operation on VM e2e-node/100") {
		t.Fatalf("vm start authorization error = %v, want canonical VM casing", err)
	}
}

func TestRunVMDeleteRejectsRunningVMBeforeMutation(t *testing.T) {
	seedLogE2E(t)
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"--yes", "--force", "--confirm-target", "e2e-node/100",
		"vm", "delete", "e2e-node/100",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "stop it first") {
		t.Fatalf("vm delete error = %v, want running-state stop remedy", err)
	}
	var exitErr *app.ExitCoder
	if !stderrors.As(err, &exitErr) || exitErr.ExitCode != app.ExitConflict {
		t.Fatalf("vm delete error = %v, want conflict exit", err)
	}
}
